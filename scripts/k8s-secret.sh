#!/usr/bin/env bash
# Create or update the Kubernetes secret from environment variables (never from git).
set -euo pipefail

: "${DATABASE_URL:?set DATABASE_URL}"
: "${REDIS_ADDR:?set REDIS_ADDR}"
: "${INGEST_TOKEN:?set INGEST_TOKEN}"

kubectl create secret generic fleet-secrets \
  --from-literal=DATABASE_URL="$DATABASE_URL" \
  --from-literal=REDIS_ADDR="$REDIS_ADDR" \
  --from-literal=INGEST_TOKEN="$INGEST_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -
