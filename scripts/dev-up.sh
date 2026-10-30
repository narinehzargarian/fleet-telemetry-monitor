#!/usr/bin/env bash
# Start the whole stack locally with Docker Compose and wait until the API is ready.
set -euo pipefail

cd "$(dirname "$0")/.."

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

export INGEST_TOKEN="${INGEST_TOKEN:-local-dev-token}"
docker compose up --build -d

echo -n "waiting for API"
for _ in $(seq 1 60); do
  if curl -fs http://localhost:8080/readyz >/dev/null; then
    echo " ready"
    echo "dashboard  http://localhost:5173"
    echo "grafana    http://localhost:3000"
    echo "prometheus http://localhost:9090"
    exit 0
  fi
  echo -n "."
  sleep 2
done

echo " API did not become ready; recent logs:" >&2
docker compose logs --tail=50 api >&2
exit 1
