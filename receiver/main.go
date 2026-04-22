package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof" // 🆕 Profiling: /debug/pprof/
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"            // go get github.com/go-chi/chi/v5
	"github.com/go-chi/chi/v5/middleware" // 🆕 Router + middleware
	// 🆕 Auth (опционально)
	"github.com/gorilla/handlers"     // CORS
	"github.com/jackc/pgx/v5/pgxpool" // 🆕 DB pool (симуляция)
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sony/gobreaker" // 🆕 Circuit breaker
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "receiver_requests_total", Help: "Total requests"},
		[]string{"status", "method", "endpoint"}, // 🆕 +endpoint
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "receiver_requests_duration_seconds",
			Help:    "Request duration",
			Buckets: prometheus.LinearBuckets(0.01, 0.02, 10),
		},
		[]string{"endpoint"}, // 🆕 Per-endpoint
	)
	dbQueries = prometheus.NewCounterVec( // 🆕 DB метрики
		prometheus.CounterOpts{Name: "receiver_db_queries_total", Help: "DB queries"},
		[]string{"query_type"},
	)
	cbState = prometheus.NewGauge(prometheus.GaugeOpts{ // 🆕 Circuit breaker
		Name: "receiver_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=open)",
	})
)

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration, dbQueries, cbState)
}

type Receiver struct {
	cb      *gobreaker.CircuitBreaker
	dbPool  *pgxpool.Pool // Симуляция DB
	rateLim *RateLimiter
}

func NewReceiver() *Receiver {
	// 🆕 Circuit breaker
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "db-breaker",
		MaxRequests: 5,
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbState.Set(float64(to))
		},
	})

	// 🆕 Rate limiter (100 RPS)
	rateLim := NewRateLimiter(100)

	// 🆕 DB pool (симуляция)
	dbPool, _ := pgxpool.New(context.Background(), "postgres://user:pass@localhost/db")

	return &Receiver{cb: cb, dbPool: dbPool, rateLim: rateLim}
}

func (r *Receiver) loadHandler(w http.ResponseWriter, req *http.Request) {
	start := prometheus.NewTimer(requestDuration.WithLabelValues("load"))
	defer start.ObserveDuration()

	// 🆕 Rate limiting
	if !r.rateLim.Allow() {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		requestsTotal.WithLabelValues("429", req.Method, "load").Inc()
		return
	}

	// 🎯 "DB" задержка с circuit breaker
	ctx := req.Context()
	state, _ := r.cb.Execute(func() (interface{}, error) {
		dbQueries.WithLabelValues("SELECT").Inc()
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(100))) // Fake DB
		return nil, nil
	})
	if state == nil {
		// Circuit open - fallback
		time.Sleep(50 * time.Millisecond)
	}

	// 🎲 Load simulation
	delay := rand.Intn(200)
	time.Sleep(time.Millisecond * time.Duration(delay))

	statusCodes := []int{200, 200, 200, 500, 503, 404}
	status := statusCodes[rand.Intn(len(statusCodes))]

	requestsTotal.WithLabelValues(fmt.Sprintf("%d", status), req.Method, "load").Inc()

	w.WriteHeader(status)
	fmt.Fprintf(w, "Status: %d | Method: %s | CB: %v | Duration: %.2fms\n",
		status, req.Method, r.cb.ReadyToGo(), time.Since(start)*1000)
}

func (r *Receiver) healthHandler(w http.ResponseWriter, req *http.Request) {
	requestDuration.WithLabelValues("health").Observe(0.001) // Fast
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK | CB:", r.cb.ReadyToGo())
}

func (r *Receiver) metricsAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		token := req.Header.Get("Authorization")
		if token != "Bearer secret-metrics" { // 🆕 Basic auth
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func main() {
	rand.Seed(time.Now().UnixNano())
	receiver := NewReceiver()

	// 🆕 Chi router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(handlers.CORS(handlers.AllowedOrigins([]string{"*"})))

	r.Get("/load", receiver.loadHandler)
	r.Post("/load", receiver.loadHandler) // 🆕 POST support
	r.Get("/health", receiver.healthHandler)
	r.Handle("/metrics", receiver.metricsAuth(promhttp.Handler()))

	// 🆕 Pprof на отдельном порту
	go func() {
		log.Println("Pprof: http://localhost:6060/debug/pprof/")
		log.Fatal(http.ListenAndServe("localhost:6060", nil))
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{Addr: ":" + port}
	idleConnsClosed := make(chan struct{})

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	fmt.Printf("🌐 Receiver v2.0 on :%s | /load /health /metrics\n", port)
	fmt.Println("🔍 Pprof: localhost:6060/debug/pprof/ | Auth: Bearer secret-metrics")
	fmt.Println("💡 Test: curl -H 'Authorization: Bearer secret-metrics' localhost:8080/metrics")

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe: %v", err)
	}
	<-idleConnsClosed
}

// 🆕 Rate limiter (token bucket)
type RateLimiter struct {
	tokens     int64
	lastRefill time.Time
	maxTokens  int64
	refillRate int64
}

func NewRateLimiter(rps int) *RateLimiter {
	return &RateLimiter{
		tokens:     int64(rps),
		maxTokens:  int64(rps),
		refillRate: int64(rps),
		lastRefill: time.Now(),
	}
}

func (rl *RateLimiter) Allow() bool {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	addTokens := int64(elapsed) * rl.refillRate

	atomic.AddInt64(&rl.tokens, addTokens)
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if atomic.LoadInt64(&rl.tokens) > 0 {
		atomic.AddInt64(&rl.tokens, -1)
		return true
	}
	return false
}
