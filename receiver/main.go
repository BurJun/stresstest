package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "receiver_requests_total",
			Help: "Total number of requests received",
		},
		[]string{"status", "method"}, // 🆕 Добавлен label "method"
	)
	requestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "receiver_requests_duration_seconds",
			Help:    "Duration of requests handling",
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

	// 🎯 Симуляция нагрузки (0-200ms)
	delay := rand.Intn(200)
	time.Sleep(time.Millisecond * time.Duration(delay))

	// 🎲 Рандомные статусы (80% OK, 20% ошибки)
	statusCodes := []int{200, 200, 200, 500, 503, 404}
	status := statusCodes[rand.Intn(len(statusCodes))]

	// 📊 Метрики с label method (GET/POST/etc.)
	requestsTotal.WithLabelValues(fmt.Sprintf("%d", status), r.Method).Inc()
	requestDuration.Observe(time.Since(start).Seconds())

	w.WriteHeader(status)
	fmt.Fprintf(w, "Status: %d | Method: %s | Duration: %.2fms\n",
		status, r.Method, time.Since(start).Seconds()*1000)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// 🆕 Новая фича: /health для Kubernetes/readiness
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// 🛤️ Роуты
	http.HandleFunc("/load", loadHandler)
	http.HandleFunc("/health", healthHandler) // 🆕
	http.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 🚀 Graceful shutdown
	srv := &http.Server{Addr: ":" + port}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	fmt.Printf("Receiver is running on http://localhost:%s\n", port)
	fmt.Println("Endpoints: /load /health /metrics")

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server ListenAndServe: %v", err)
	}
	<-idleConnsClosed
}
