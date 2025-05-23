package main

import (
	"flag"
	"fmt"
	"net/http"
	"sync"
	"time"
)

func main() {
	target := flag.String("url", "http://localhost:8080/load", "Target URL to bombard")
	rps := flag.Int("rps", 100, "Request per second")
	duration := flag.Int("duration", 10, "Duration of attack in seconds")
	flag.Parse()

	fmt.Printf("Attacking %s with %d rps for %d seconds...\n", *target, *rps, *duration)

	var wg sync.WaitGroup
	end := time.Now().Add(time.Duration(*duration) * time.Second)
	ticker := time.NewTicker(time.Second / time.Duration(*rps))

	for time.Now().Before(end) {
		<-ticker.C
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			resp, err := http.Get(*target)
			elapsed := time.Since(start)

			if err != nil {
				fmt.Printf("[err] %v\n", err)
				return
			}
			defer resp.Body.Close()
			fmt.Printf("[ok] %d (%v)\n", resp.StatusCode, elapsed)
		}()
	}
	ticker.Stop()
	wg.Wait()
	fmt.Printf("Attack complete.")
}
