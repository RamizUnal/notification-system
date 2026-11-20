#!/usr/bin/env bash
# Sends a mixed burst of 1,000 synthetic notifications to a running stack.
# Watch it drain at the per-channel ceiling on http://localhost:8070.
set -euo pipefail

API=${API:-http://localhost:8080}
COUNT=${COUNT:-1000}
tag=$(date +%H%M%S)

channels=(sms email push)
recipients=("+905551234567" "ada@example.com" "fcm:APA91bH-demo-device-01")
priorities=(high normal normal low)

body='{"notifications":['
for ((i = 0; i < COUNT; i++)); do
  c=$((i % 3))
  [ "$i" -gt 0 ] && body+=','
  body+="{\"channel\":\"${channels[$c]}\",\"recipient\":\"${recipients[$c]}\",\"priority\":\"${priorities[$((i % 4))]}\",\"content\":\"Flash sale: 40% off for 2 hours (demo $tag-$i)\"}"
done
body+=']}'

response=$(curl -fsS -X POST "$API/api/v1/batches" -H 'Content-Type: application/json' --data-binary "$body")
batch=$(printf '%s' "$response" | sed -E 's/.*"id":"([^"]+)".*/\1/')
echo "Queued $COUNT notifications as batch $batch"
echo "Follow it:  http://localhost:8070/batches/$batch"
echo "Or poll:    curl -s $API/api/v1/batches/$batch"
