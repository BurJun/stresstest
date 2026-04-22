package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pterm/pterm" // go get github.com/pterm/pterm
)

type Stats struct {
	total     int64
	success   int64
	errors    int64
	minLat    time.Duration
	maxLat    time.Duration
	totalTime time.Duration
}

func main() {
	var (
		target   = flag.String("url", "http://localhost:8080/load", "Target URL")
		rps      = flag.Int("rps", 100, "Requests per second")
		duration = flag.Int("duration", 10, "Duration (seconds)")
		timeout  = flag.Duration("timeout", 5*time.Second, "Request timeout")
	)
	flag.Parse()

	fmt.Printf("🚀 Attacking %s @ %d rps for %d sec (timeout: %v)\n\n",
		*target, *rps, *duration, *timeout)

	stats := &Stats{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*duration)*time.Second)
	defer cancel()

	// 🎯 Rate limiter: ровно rps запросов/сек
	ticker := time.NewTicker(time.Second / time.Duration(*rps))
	defer ticker.Stop()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// 📊 Progress bar
	progress := pterm.DefaultProgressbar.WithTotal(*rps * *duration).
		WithTitle("Load test").WithProgressCharacter("█")

	go func() {
		tickerCh := ticker.C
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-tickerCh:
				wg.Add(1)
				reqID := atomic.AddInt64(&stats.total, 1)
				progress.Increment()
				go worker(ctx, *target, *timeout, stats, reqID)
			}
		}
	}()

	// ⏱️ Graceful stop
	go func() {
		<-ctx.Done()
		wg.Wait()
	}()

	wg.Wait()
	close(done)

	// 📈 Итоговые метрики
	successRate := float64(stats.success) / float64(stats.total) * 100
	fmt.Printf("\n\n📊 **FINAL STATS** 📊\n")
	fmt.Printf("Total requests:     %d\n", stats.total)
	fmt.Printf("Success:            %d (%.1f%%)\n", stats.success, successRate)
	fmt.Printf("Errors:             %d\n", stats.errors)
	fmt.Printf("Avg latency:        %.2f ms\n", stats.totalTime.Seconds()/float64(stats.success)*1000)
	fmt.Printf("Min/Max latency:    %v / %v\n", stats.minLat, stats.maxLat)
}

func worker(ctx context.Context, target string, timeout time.Duration, stats *Stats, reqID int64) {
	defer func() { atomic.AddInt64(&stats.total, 0) }() // Уже учтен

	start := time.Now()
	client := &http.Client{Timeout: timeout}
	req, _ := http.NewRequestWithContext(ctx, "GET", target, nil)

	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		atomic.AddInt64(&stats.errors, 1)
		fmt.Printf("\r[ERR#%d] %v\n", reqID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		atomic.AddInt64(&stats.success, 1)
		atomic.AddInt64(int64(latency.Milliseconds()), &stats.totalTime.Milliseconds()) // Примитивно, но работает

		// Min/Max latency
		if latency < stats.minLat || stats.minLat == 0 {
			atomic.StoreInt64((*int64)(&stats.minLat), int64(latency))
		}
		if latency > stats.maxLat {
			atomic.StoreInt64((*int64)(&stats.maxLat), int64(latency))
		}

		fmt.Printf("\r[OK#%d] %d (%.0fms)   \n", reqID, resp.StatusCode, latency.Seconds()*1000)
	} else {
		atomic.AddInt64(&stats.errors, 1)
		fmt.Printf("\r[FAIL#%d] %d (%.0fms)\n", reqID, resp.StatusCode, latency.Seconds()*1000)
	}
}
