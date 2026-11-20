#!/usr/bin/env bash
# Smoke test for a running stack: every service answers, and one
# notification travels API -> queue -> worker -> provider -> sent.
set -euo pipefail

API=${API:-http://localhost:8080}
CONSOLE=${CONSOLE:-http://localhost:8070}

check() {
  local name=$1 url=$2
  if curl -fsS -o /dev/null "$url"; then
    echo "ok    $name ($url)"
  else
    echo "FAIL  $name ($url)" >&2
    exit 1
  fi
}

check "API readiness" "$API/readyz"
check "OpenAPI spec" "$API/openapi.yaml"
check "Console" "$CONSOLE/"
check "Console API proxy" "$CONSOLE/api/v1/metrics"

response=$(curl -fsS -X POST "$API/api/v1/notifications" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: smoke-$(date +%s)" \
  -d '{"channel":"sms","recipient":"+905551234567","content":"Smoke test","priority":"high"}')
id=$(printf '%s' "$response" | sed -E 's/.*"id":"([^"]+)".*/\1/')
echo "ok    created notification $id"

for _ in $(seq 1 60); do
  status=$(curl -fsS "$API/api/v1/notifications/$id" | sed -E 's/.*"status":"([^"]+)".*/\1/')
  if [ "$status" = "sent" ]; then
    echo "ok    notification delivered"
    exit 0
  fi
  sleep 0.5
done

echo "FAIL  notification $id is still '$status' after 30s" >&2
exit 1
