package rules

import (
	"testing"
	"time"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

func healthy() telemetry.Reading {
	return telemetry.Reading{
		ReadingID: "r1", StationID: "gs-01", Timestamp: time.Now(),
		CPUTempC: 50, SignalSNRdB: 20, DiskUsedPct: 40, Online: true,
	}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*telemetry.Reading)
		want   map[string]Severity // rule -> severity
	}{
		{"healthy reading has no alerts", func(r *telemetry.Reading) {}, map[string]Severity{}},
		{"cpu warning", func(r *telemetry.Reading) { r.CPUTempC = 80 }, map[string]Severity{"cpu_temp": Warning}},
		{"cpu critical wins over warning", func(r *telemetry.Reading) { r.CPUTempC = 95 }, map[string]Severity{"cpu_temp": Critical}},
		{"low signal", func(r *telemetry.Reading) { r.SignalSNRdB = 5 }, map[string]Severity{"low_signal": Warning}},
		{"disk nearly full", func(r *telemetry.Reading) { r.DiskUsedPct = 90 }, map[string]Severity{"disk_usage": Warning}},
		{"multiple problems at once", func(r *telemetry.Reading) { r.CPUTempC = 80; r.SignalSNRdB = 2 },
			map[string]Severity{"cpu_temp": Warning, "low_signal": Warning}},
		{"offline suppresses other rules", func(r *telemetry.Reading) { r.Online = false; r.CPUTempC = 99 },
			map[string]Severity{"station_offline": Critical}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := healthy()
			tc.modify(&r)
			got := Evaluate(r, Default())
			if len(got) != len(tc.want) {
				t.Fatalf("got %d alerts %+v, want %d", len(got), got, len(tc.want))
			}
			for _, alert := range got {
				wantSeverity, ok := tc.want[alert.Rule]
				if !ok {
					t.Errorf("unexpected alert %q", alert.Rule)
					continue
				}
				if alert.Severity != wantSeverity {
					t.Errorf("rule %q: got severity %s, want %s", alert.Rule, alert.Severity, wantSeverity)
				}
			}
		})
	}
}
