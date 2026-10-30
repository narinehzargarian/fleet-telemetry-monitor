// Package store persists readings and alerts in PostgreSQL.
package store

import (
	"context"
	_ "embed"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/rules"
	"github.com/narinehzargarian/fleet-telemetry-monitor/internal/telemetry"
)

//go:embed schema.sql
var schema string

// Store wraps a Postgres connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// schemaLockID is an arbitrary constant for pg_advisory_lock.
const schemaLockID = 482_901

// New connects to Postgres and applies the schema.
//
// Several replicas (API and processor pods) start at the same time, and concurrent
// CREATE TABLE IF NOT EXISTS calls can still collide in Postgres's catalog. An
// advisory lock makes the migration run one process at a time.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", schemaLockID); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", schemaLockID)

	_, err = conn.Exec(ctx, schema)
	return err
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping checks the database is reachable (used by readiness checks).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// SaveReading inserts a reading. It returns inserted=false when the reading_id
// already exists, which makes redelivered stream messages harmless.
func (s *Store) SaveReading(ctx context.Context, r telemetry.Reading) (inserted bool, err error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO readings (reading_id, station_id, ts, cpu_temp_c, signal_snr_db, disk_used_pct, online)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (reading_id) DO NOTHING`,
		r.ReadingID, r.StationID, r.Timestamp, r.CPUTempC, r.SignalSNRdB, r.DiskUsedPct, r.Online)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// SaveAlerts stores the alerts produced for one reading.
func (s *Store) SaveAlerts(ctx context.Context, alerts []rules.Alert) error {
	for _, a := range alerts {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO alerts (station_id, reading_id, rule, severity, message)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (reading_id, rule) DO NOTHING`,
			a.StationID, a.ReadingID, a.Rule, string(a.Severity), a.Message); err != nil {
			return err
		}
	}
	return nil
}

// StationStatus is the latest health snapshot for one station.
type StationStatus struct {
	StationID   string    `json:"station_id"`
	LastSeen    time.Time `json:"last_seen"`
	CPUTempC    float64   `json:"cpu_temp_c"`
	SignalSNRdB float64   `json:"signal_snr_db"`
	DiskUsedPct float64   `json:"disk_used_pct"`
	Online      bool      `json:"online"`
	OpenAlerts  int       `json:"open_alerts"`
}

// Stations returns the most recent reading for every station plus its open alert count.
func (s *Store) Stations(ctx context.Context) ([]StationStatus, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (r.station_id)
			r.station_id, r.ts, r.cpu_temp_c, r.signal_snr_db, r.disk_used_pct, r.online,
			(SELECT count(*) FROM alerts a WHERE a.station_id = r.station_id AND NOT a.resolved)
		FROM readings r
		ORDER BY r.station_id, r.ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statuses := []StationStatus{}
	for rows.Next() {
		var st StationStatus
		if err := rows.Scan(&st.StationID, &st.LastSeen, &st.CPUTempC, &st.SignalSNRdB,
			&st.DiskUsedPct, &st.Online, &st.OpenAlerts); err != nil {
			return nil, err
		}
		statuses = append(statuses, st)
	}
	return statuses, rows.Err()
}

// AlertRecord is an alert as stored, with its id and time.
type AlertRecord struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	rules.Alert
}

// RecentAlerts returns the newest unresolved alerts.
func (s *Store) RecentAlerts(ctx context.Context, limit int) ([]AlertRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, created_at, station_id, reading_id, rule, severity, message
		FROM alerts WHERE NOT resolved
		ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []AlertRecord{}
	for rows.Next() {
		var rec AlertRecord
		var severity string
		if err := rows.Scan(&rec.ID, &rec.CreatedAt, &rec.StationID, &rec.ReadingID,
			&rec.Rule, &severity, &rec.Message); err != nil {
			return nil, err
		}
		rec.Severity = rules.Severity(severity)
		records = append(records, rec)
	}
	return records, rows.Err()
}

// ResolveAlert marks an alert as handled. It returns false if no open alert had that id.
func (s *Store) ResolveAlert(ctx context.Context, id int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE alerts SET resolved = TRUE WHERE id = $1 AND NOT resolved`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
