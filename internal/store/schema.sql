CREATE TABLE IF NOT EXISTS readings (
    reading_id     TEXT PRIMARY KEY,           -- idempotency key from the station
    station_id     TEXT        NOT NULL,
    ts             TIMESTAMPTZ NOT NULL,
    cpu_temp_c     DOUBLE PRECISION NOT NULL,
    signal_snr_db  DOUBLE PRECISION NOT NULL,
    disk_used_pct  DOUBLE PRECISION NOT NULL,
    online         BOOLEAN     NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "Latest reading per station" is the dashboard's hot query.
CREATE INDEX IF NOT EXISTS readings_station_ts_idx ON readings (station_id, ts DESC);

CREATE TABLE IF NOT EXISTS alerts (
    id          BIGSERIAL PRIMARY KEY,
    station_id  TEXT        NOT NULL,
    reading_id  TEXT        NOT NULL REFERENCES readings (reading_id),
    rule        TEXT        NOT NULL,
    severity    TEXT        NOT NULL CHECK (severity IN ('warning', 'critical')),
    message     TEXT        NOT NULL,
    resolved    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (reading_id, rule)                  -- reprocessing a reading never duplicates alerts
);

CREATE INDEX IF NOT EXISTS alerts_open_idx ON alerts (created_at DESC) WHERE NOT resolved;
