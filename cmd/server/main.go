// Command server is the REST API: it accepts telemetry from stations, queues it,
// and serves fleet status and alerts to the dashboard.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/metrics"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/store"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/stream"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

type server struct {
	db       *store.Store
	rdb      *redis.Client
	pub      *stream.Publisher
	apiToken string // shared secret stations must send; empty disables auth (local dev only)
	log      *slog.Logger
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, env("DATABASE_URL", "postgres://fleet:fleet@localhost:5432/fleet?sslmode=disable"))
	if err != nil {
		log.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	defer rdb.Close()

	s := &server{db: db, rdb: rdb, pub: stream.NewPublisher(rdb), apiToken: os.Getenv("INGEST_TOKEN"), log: log}

	httpServer := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("api listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	// Graceful shutdown: finish in-flight requests before exiting (matters during rolling deploys).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Info("api stopped")
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/telemetry", s.ingest)
	mux.HandleFunc("GET /api/v1/stations", s.stations)
	mux.HandleFunc("GET /api/v1/alerts", s.alerts)
	mux.HandleFunc("POST /api/v1/alerts/{id}/resolve", s.resolveAlert)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", s.ready)
	mux.Handle("GET /metrics", promhttp.Handler())
	return withCORS(mux)
}

// ingest validates a reading and queues it. It returns 202 because storage happens asynchronously.
func (s *server) ingest(w http.ResponseWriter, r *http.Request) {
	if s.apiToken != "" && r.Header.Get("Authorization") != "Bearer "+s.apiToken {
		writeError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	var reading telemetry.Reading
	body := http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(body).Decode(&reading); err != nil {
		metrics.ReadingsRejected.Inc()
		writeError(w, http.StatusBadRequest, "body must be a JSON reading")
		return
	}
	if err := reading.Validate(time.Now()); err != nil {
		metrics.ReadingsRejected.Inc()
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.pub.Publish(r.Context(), reading); err != nil {
		s.log.Error("publish reading", "err", err, "station", reading.StationID)
		writeError(w, http.StatusServiceUnavailable, "queue unavailable, retry later")
		return
	}
	metrics.ReadingsIngested.Inc()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "reading_id": reading.ReadingID})
}

func (s *server) stations(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.Stations(r.Context())
	if err != nil {
		s.log.Error("list stations", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load stations")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) alerts(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	list, err := s.db.RecentAlerts(r.Context(), limit)
	if err != nil {
		s.log.Error("list alerts", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load alerts")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) resolveAlert(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "alert id must be a number")
		return
	}
	ok, err := s.db.ResolveAlert(r.Context(), id)
	if err != nil {
		s.log.Error("resolve alert", "err", err, "id", id)
		writeError(w, http.StatusInternalServerError, "could not resolve alert")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no open alert with that id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// ready reports whether both dependencies are reachable, so Kubernetes only routes traffic to healthy pods.
func (s *server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "postgres unreachable")
		return
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "redis unreachable")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
