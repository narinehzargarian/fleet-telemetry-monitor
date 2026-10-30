// Command processor consumes readings from the Redis Stream, stores them,
// runs the alert rules, and updates Prometheus metrics.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/metrics"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/rules"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/store"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/stream"
)

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

	hostname, _ := os.Hostname()
	consumer, err := stream.NewConsumer(ctx, rdb, env("CONSUMER_NAME", hostname))
	if err != nil {
		log.Error("create consumer", "err", err)
		os.Exit(1)
	}

	// Serve /metrics so Prometheus can scrape the processor too.
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		addr := ":" + env("METRICS_PORT", "9091")
		log.Info("processor metrics listening", "addr", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Error("metrics server", "err", err)
		}
	}()

	thresholds := rules.Default()
	log.Info("processor started")
	for ctx.Err() == nil {
		msgs, err := consumer.Read(ctx, 50, 2*time.Second)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}
			log.Error("read stream", "err", err)
			time.Sleep(time.Second) // back off instead of spinning while Redis is down
			continue
		}
		for _, m := range msgs {
			if err := handle(ctx, db, m, thresholds, log); err != nil {
				// Leave it unacknowledged; it will be retried on the next pass.
				log.Error("process reading", "err", err, "msg_id", m.ID)
				continue
			}
			if err := consumer.Ack(ctx, m.ID); err != nil {
				log.Error("ack", "err", err, "msg_id", m.ID)
			}
		}
	}
	log.Info("processor stopped")
}

func handle(ctx context.Context, db *store.Store, m stream.Message, t rules.Thresholds, log *slog.Logger) error {
	if m.Malformed {
		metrics.ReadingsProcessed.WithLabelValues("malformed").Inc()
		log.Warn("dropping malformed message", "msg_id", m.ID)
		return nil // ack it so it does not block the stream forever
	}
	start := time.Now()
	r := m.Reading

	inserted, err := db.SaveReading(ctx, r)
	if err != nil {
		metrics.ReadingsProcessed.WithLabelValues("error").Inc()
		return err
	}
	if !inserted {
		metrics.ReadingsProcessed.WithLabelValues("duplicate").Inc()
		return nil
	}

	alerts := rules.Evaluate(r, t)
	if err := db.SaveAlerts(ctx, alerts); err != nil {
		metrics.ReadingsProcessed.WithLabelValues("error").Inc()
		return err
	}
	for _, a := range alerts {
		metrics.AlertsRaised.WithLabelValues(a.Rule, string(a.Severity)).Inc()
		log.Warn("alert", "station", a.StationID, "rule", a.Rule, "severity", a.Severity, "message", a.Message)
	}

	metrics.StationCPUTemp.WithLabelValues(r.StationID).Set(r.CPUTempC)
	metrics.StationSignal.WithLabelValues(r.StationID).Set(r.SignalSNRdB)
	online := 0.0
	if r.Online {
		online = 1
	}
	metrics.StationOnline.WithLabelValues(r.StationID).Set(online)

	metrics.ReadingsProcessed.WithLabelValues("stored").Inc()
	metrics.ProcessingSeconds.Observe(time.Since(start).Seconds())
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
