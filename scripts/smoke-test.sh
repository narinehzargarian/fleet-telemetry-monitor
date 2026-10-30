#!/usr/bin/env bash
# Post-deploy smoke test: send one faulty reading and confirm it shows up as an alert.
# Usage: API_URL=https://fleet.example.com INGEST_TOKEN=... ./scripts/smoke-test.sh
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
TOKEN="${INGEST_TOKEN:-local-dev-token}"
ID="smoke-$(date +%s)"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

curl -fs "$API_URL/readyz" >/dev/null || { echo "FAIL: API not ready" >&2; exit 1; }

curl -fs -X POST "$API_URL/api/v1/telemetry" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"reading_id\":\"$ID\",\"station_id\":\"smoke-test\",\"timestamp\":\"$NOW\",\"cpu_temp_c\":97,\"signal_snr_db\":20,\"disk_used_pct\":20,\"online\":true}" \
  >/dev/null

for _ in $(seq 1 15); do
  if curl -fs "$API_URL/api/v1/alerts?limit=100" | grep -q "\"reading_id\":\"$ID\""; then
    echo "PASS: reading $ID was processed and raised an alert"
    exit 0
  fi
  sleep 1
done

echo "FAIL: no alert for $ID after 15s" >&2
exit 1
