// Package rules turns raw readings into alerts. It is pure logic with no I/O,
// so every threshold is easy to unit test.
package rules

import (
	"fmt"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

// Severity is how urgent an alert is.
type Severity string

const (
	Warning  Severity = "warning"
	Critical Severity = "critical"
)

// Alert is a problem detected on one reading.
type Alert struct {
	StationID string   `json:"station_id"`
	ReadingID string   `json:"reading_id"`
	Rule      string   `json:"rule"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
}

// Thresholds are configurable so operators can tune alerting per fleet.
type Thresholds struct {
	CPUTempWarnC    float64
	CPUTempCritC    float64
	MinSignalSNRdB  float64
	DiskUsedWarnPct float64
}

// Default returns thresholds that suit a typical outdoor ground station.
func Default() Thresholds {
	return Thresholds{
		CPUTempWarnC:    75,
		CPUTempCritC:    90,
		MinSignalSNRdB:  10,
		DiskUsedWarnPct: 85,
	}
}

// Evaluate returns every alert a reading triggers, or none if it is healthy.
func Evaluate(r telemetry.Reading, t Thresholds) []Alert {
	var alerts []Alert
	add := func(rule string, sev Severity, msg string) {
		alerts = append(alerts, Alert{
			StationID: r.StationID, ReadingID: r.ReadingID,
			Rule: rule, Severity: sev, Message: msg,
		})
	}

	if !r.Online {
		add("station_offline", Critical, "station reported offline")
		// Other metrics are unreliable while the station is down.
		return alerts
	}

	switch {
	case r.CPUTempC >= t.CPUTempCritC:
		add("cpu_temp", Critical, fmt.Sprintf("CPU at %.1f°C (critical >= %.0f°C)", r.CPUTempC, t.CPUTempCritC))
	case r.CPUTempC >= t.CPUTempWarnC:
		add("cpu_temp", Warning, fmt.Sprintf("CPU at %.1f°C (warning >= %.0f°C)", r.CPUTempC, t.CPUTempWarnC))
	}

	if r.SignalSNRdB < t.MinSignalSNRdB {
		add("low_signal", Warning, fmt.Sprintf("signal SNR %.1f dB below %.0f dB", r.SignalSNRdB, t.MinSignalSNRdB))
	}

	if r.DiskUsedPct >= t.DiskUsedWarnPct {
		add("disk_usage", Warning, fmt.Sprintf("disk %.0f%% full", r.DiskUsedPct))
	}

	return alerts
}
