// Package metrics exposes Prometheus metrics for the API and processor.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ReadingsIngested counts readings accepted by the API.
	ReadingsIngested = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fleet_readings_ingested_total",
		Help: "Readings accepted by the ingest API.",
	})

	// ReadingsRejected counts readings that failed validation.
	ReadingsRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fleet_readings_rejected_total",
		Help: "Readings rejected by validation.",
	})

	// ReadingsProcessed counts readings stored by the processor, labeled by outcome.
	ReadingsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fleet_readings_processed_total",
		Help: "Readings handled by the processor.",
	}, []string{"outcome"}) // stored | duplicate | malformed | error

	// AlertsRaised counts alerts by rule and severity.
	AlertsRaised = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fleet_alerts_raised_total",
		Help: "Alerts raised by the rules engine.",
	}, []string{"rule", "severity"})

	// StationCPUTemp is the latest CPU temperature per station.
	StationCPUTemp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fleet_station_cpu_temp_celsius",
		Help: "Latest CPU temperature reported by each station.",
	}, []string{"station"})

	// StationSignal is the latest signal SNR per station.
	StationSignal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fleet_station_signal_snr_db",
		Help: "Latest signal SNR reported by each station.",
	}, []string{"station"})

	// StationOnline is 1 when the station's latest reading said it was online.
	StationOnline = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fleet_station_online",
		Help: "1 if the station's latest reading was online, else 0.",
	}, []string{"station"})

	// ProcessingSeconds measures time from dequeue to stored.
	ProcessingSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "fleet_processing_seconds",
		Help:    "Time to store a reading and evaluate its alerts.",
		Buckets: prometheus.DefBuckets,
	})
)
