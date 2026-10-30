// Command simulator pretends to be a fleet of ground stations sending telemetry,
// including occasional faults (overheating, weak signal, outages) so alerts fire.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	mathrand "math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

type station struct {
	id       string
	baseTemp float64
}

func main() {
	api := flag.String("api", "http://localhost:8080", "API base URL")
	count := flag.Int("stations", 6, "number of simulated stations")
	interval := flag.Duration("interval", 2*time.Second, "time between readings per station")
	faultRate := flag.Float64("fault-rate", 0.08, "chance a reading has a fault")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stations := make([]station, *count)
	for i := range stations {
		stations[i] = station{id: fmt.Sprintf("gs-%02d", i+1), baseTemp: 45 + mathrand.Float64()*15}
	}
	client := &http.Client{Timeout: 5 * time.Second}
	token := os.Getenv("INGEST_TOKEN")

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	log.Printf("simulating %d stations -> %s", *count, *api)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, st := range stations {
				reading := next(st, *faultRate)
				if err := send(client, *api, token, reading); err != nil {
					log.Printf("%s: %v", st.id, err)
				}
			}
		}
	}
}

func next(st station, faultRate float64) telemetry.Reading {
	r := telemetry.Reading{
		ReadingID:   newID(),
		StationID:   st.id,
		Timestamp:   time.Now().UTC(),
		CPUTempC:    st.baseTemp + mathrand.NormFloat64()*3,
		SignalSNRdB: 22 + mathrand.NormFloat64()*3,
		DiskUsedPct: 40 + mathrand.Float64()*20,
		Online:      true,
	}
	if mathrand.Float64() < faultRate {
		switch mathrand.IntN(4) {
		case 0:
			r.CPUTempC = 80 + mathrand.Float64()*15 // overheating
		case 1:
			r.SignalSNRdB = mathrand.Float64() * 8 // weather or misalignment
		case 2:
			r.DiskUsedPct = 88 + mathrand.Float64()*10
		case 3:
			r.Online = false
		}
	}
	return r
}

func send(client *http.Client, api, token string, r telemetry.Reading) error {
	body, _ := json.Marshal(r)
	req, err := http.NewRequest(http.MethodPost, api+"/api/v1/telemetry", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
