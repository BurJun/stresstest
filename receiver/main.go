package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "receuver_requests_total",
			Help: "Total number of requests received",
		},
		[]string{"status"},
	)
	requestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "receuver_requests_duration_seconds",
			Help:    "Duration of requests handing",
			Buckets: prometheus.LinearBuckets(0.01, 0.02, 10),
		},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(requestDuration)
}

func loadHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	delay := rand.Intn(200)
	time.Sleep(time.Millisecond * time.Duration(delay))

	statusCodes := []int{200, 200, 200, 500, 503, 404}
	status := statusCodes[rand.Intn(len(statusCodes))]

	requestsTotal.WithLabelValues(fmt.Sprintf("%d", status)).Inc()
	requestDuration.Observe(time.Since(start).Seconds())

	w.WriteHeader(status)
	fmt.Fprintf(w, "Status: %d\n", status)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/load", loadHandler)
	http.Handle("/metrics", promhttp.Handler())

	fmt.Println("Receiver is running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
