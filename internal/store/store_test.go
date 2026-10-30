package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/rules"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

// Integration test: runs only when TEST_DATABASE_URL points at a disposable Postgres
// (CI provides one as a service container).
func TestSaveReadingIsIdempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r := telemetry.Reading{
		ReadingID: "it-" + time.Now().Format("150405.000000000"), StationID: "gs-it",
		Timestamp: time.Now(), CPUTempC: 95, SignalSNRdB: 20, DiskUsedPct: 10, Online: true,
	}

	first, err := s.SaveReading(ctx, r)
	if err != nil || !first {
		t.Fatalf("first insert: inserted=%v err=%v", first, err)
	}
	second, err := s.SaveReading(ctx, r)
	if err != nil || second {
		t.Fatalf("redelivery should be ignored: inserted=%v err=%v", second, err)
	}

	alerts := rules.Evaluate(r, rules.Default())
	for range 2 { // saving the same alerts twice must not duplicate them
		if err := s.SaveAlerts(ctx, alerts); err != nil {
			t.Fatal(err)
		}
	}
	open, err := s.RecentAlerts(ctx, 500)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, a := range open {
		if a.ReadingID == r.ReadingID {
			count++
		}
	}
	if count != len(alerts) {
		t.Fatalf("got %d stored alerts, want %d", count, len(alerts))
	}
}
