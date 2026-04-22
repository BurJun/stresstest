package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pterm/pterm" // go get github.com/pterm/pterm
)

type Stats struct {
	total       int64
	success     int64
	errors      int64
	statusCodes map[int]int64 // 🆕 Счетчики по статусам
	latencies   []time.Duration
	minLat      time.Duration
	maxLat      time.Duration
	totalTime   time.Duration
	mu          sync.RWMutex // 🆕 Для thread-safe latencies
}

func main() {
	var (
		target    = flag.String("url", "http://localhost:8080/load", "Target URL")
		rps       = flag.Int("rps", 100, "Requests per second")
		duration  = flag.Int("duration", 10, "Duration (seconds)")
		timeout   = flag.Duration("timeout", 5*time.Second, "Request timeout")
		output    = flag.String("output", "", "CSV output file")
		dashboard = flag.Bool("dashboard", true, "Real-time dashboard")
	)
	flag.Parse()

	fmt.Printf("🚀 Load test: %s @ %d RPS / %d sec\n\n", *target, *rps, *duration)

	stats := &Stats{
		statusCodes: make(map[int]int64),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*duration)*time.Second)
	defer cancel()

	ticker := time.NewTicker(time.Second / time.Duration(*rps))
	defer ticker.Stop()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// 🆕 Real-time dashboard каждую секунду
	if *dashboard {
		go printDashboard(stats, time.Tick(time.Second))
	}

	// 🎯 Rate-limited workers
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case <-ticker.C:
				wg.Add(1)
				reqID := atomic.AddInt64(&stats.total, 1)
				go worker(ctx, *target, *timeout, stats, reqID)
			}
		}
	}()

	wg.Wait()
	<-ctx.Done()

	// 📊 Final report
	printFinalReport(stats)

	// 🆕 CSV export
	if *output != "" {
		exportCSV(stats, *output)
	}
}

func worker(ctx context.Context, target string, timeout time.Duration, stats *Stats, reqID int64) {
	defer wg.Done()

	start := time.Now()
	client := &http.Client{Timeout: timeout}
	req, _ := http.NewRequestWithContext(ctx, "GET", target, nil)

	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		stats.mu.Lock()
		stats.errors++
		stats.mu.Unlock()
		pterm.Error.Printf("ERR#%d: %v", reqID, err)
		return
	}
	defer resp.Body.Close()

	stats.mu.Lock()
	defer stats.mu.Unlock()

	// 🆕 Status codes
	stats.statusCodes[resp.StatusCode]++

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		stats.success++
		stats.totalTime += latency
		stats.latencies = append(stats.latencies, latency)

		// Min/Max
		if latency < stats.minLat || stats.minLat == 0 {
			stats.minLat = latency
		}
		if latency > stats.maxLat {
			stats.maxLat = latency
		}
	} else {
		stats.errors++
	}
}

func printDashboard(stats *Stats, ticker <-chan time.Time) {
	for range ticker {
		stats.mu.RLock()
		current := atomic.LoadInt64(&stats.total)
		successRate := float64(stats.success) / float64(current) * 100
		stats.mu.RUnlock()

		pterm.DefaultBigText.WithLetters(
			pterm.DefaultBigText.Letters.FromString(fmt.Sprintf("%.0f RPS", float64(current)/10)),
		).Render()

		pterm.DefaultProgress.WithCurrent(int64(float64(current) / float64(10000) * 100)).Render()
	}
}

func printFinalReport(stats *Stats) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	total := int64(len(stats.latencies)) + stats.errors
	if total == 0 {
		return
	}

	sort.Slice(stats.latencies, func(i, j int) bool {
		return stats.latencies[i] < stats.latencies[j]
	})

	p99 := stats.latencies[int(0.99*float64(len(stats.latencies)))]
	avg := stats.totalTime.Seconds() / float64(stats.success) * 1000

	pterm.DefaultHeader.WithFullWidth().Println("📊 LOAD TEST RESULTS")
	fmt.Printf("Total requests:  %d\n", total)
	fmt.Printf("Success:         %d (%.1f%%)\n", stats.success, float64(stats.success)/float64(total)*100)
	fmt.Printf("Errors:          %d\n", stats.errors)
	fmt.Printf("Avg latency:     %.0fms\n", avg)
	fmt.Printf("P99 latency:     %.0fms\n", p99.Seconds()*1000)
	fmt.Printf("Min/Max:         %v / %v\n", stats.minLat, stats.maxLat)

	fmt.Print("\nStatus codes:\n")
	for code, count := range stats.statusCodes {
		fmt.Printf("  %d: %d (%.1f%%)\n", code, count, float64(count)/float64(total)*100)
	}
}

func exportCSV(stats *Stats, filename string) {
	stats.mu.RLock()
	defer stats.mu.RUnlock()

	file, _ := os.Create(filename)
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Write([]string{"metric", "value"})

	total := int64(len(stats.latencies)) + stats.errors
	writer.Write([]string{"total_requests", fmt.Sprint(total)})
	writer.Write([]string{"success", fmt.Sprint(stats.success)})
	writer.Write([]string{"errors", fmt.Sprint(stats.errors)})
	writer.Write([]string{"avg_latency_ms", fmt.Sprint(stats.totalTime.Seconds() / float64(stats.success) * 1000)})
	writer.Write([]string{"p99_latency_ms", fmt.Sprint(time.Duration(0.99*float64(len(stats.latencies))) * time.Millisecond)})

	writer.Flush()
	fmt.Printf("💾 CSV saved: %s\n", filename)
}
