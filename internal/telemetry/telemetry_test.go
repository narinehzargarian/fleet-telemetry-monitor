package telemetry

import (
	"errors"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	valid := Reading{ReadingID: "r1", StationID: "gs-01", Timestamp: now, CPUTempC: 40, DiskUsedPct: 30, Online: true}

	cases := []struct {
		name    string
		modify  func(*Reading)
		wantErr bool
	}{
		{"valid", func(r *Reading) {}, false},
		{"missing reading id", func(r *Reading) { r.ReadingID = "" }, true},
		{"missing station id", func(r *Reading) { r.StationID = "" }, true},
		{"missing timestamp", func(r *Reading) { r.Timestamp = time.Time{} }, true},
		{"far future timestamp", func(r *Reading) { r.Timestamp = now.Add(time.Hour) }, true},
		{"small clock skew is fine", func(r *Reading) { r.Timestamp = now.Add(time.Minute) }, false},
		{"impossible cpu temp", func(r *Reading) { r.CPUTempC = 500 }, true},
		{"disk over 100 percent", func(r *Reading) { r.DiskUsedPct = 120 }, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := valid
			tc.modify(&r)
			err := r.Validate(now)
			if tc.wantErr && !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
