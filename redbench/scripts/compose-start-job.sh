#!/bin/sh
set -eu

log() { echo "job_init: $*"; }

CONTROLLER_URL="${CONTROLLER_URL:-http://controller:8081}"
EXPECTED_WORKERS="${EXPECTED_WORKERS:-3}"
MAX_WAIT_SECS="${MAX_WAIT_SECS:-120}"
SLEEP_SECS="${SLEEP_SECS:-2}"

start_ts=$(date +%s)

log "waiting for controller at $CONTROLLER_URL"
while :; do
  code=$(curl -s -o /dev/null -w "%{http_code}" "$CONTROLLER_URL/health" || true)
  if [ "$code" = "200" ]; then
    break
  fi
  now=$(date +%s)
  if [ $((now - start_ts)) -ge "$MAX_WAIT_SECS" ]; then
    log "timeout waiting for controller health"
    exit 1
  fi
  sleep "$SLEEP_SECS"
done

log "controller is healthy; waiting for $EXPECTED_WORKERS workers to register"
while :; do
  resp=$(curl -s "$CONTROLLER_URL/workers" || true)
  # Extract numeric value of "total" using sed; fallback to 0 if parse fails
  total=$(echo "$resp" | tr -d '\n' | sed -e 's/.*"total"[[:space:]]*:[[:space:]]*//' -e 's/[^0-9].*//' )
  case "$total" in
    ('' ) total=0 ;;
  esac
  if [ "$total" -ge "$EXPECTED_WORKERS" ]; then
    break
  fi
  now=$(date +%s)
  if [ $((now - start_ts)) -ge "$MAX_WAIT_SECS" ]; then
    log "timeout waiting for workers (got ${total}/$EXPECTED_WORKERS)"
    exit 1
  fi
  sleep "$SLEEP_SECS"
done

log "workers ready; starting coordinated job"
payload='{
  "targets": [
    {"redisUrl": "redis://redis8:6379", "workerCount": 1},
    {"redisUrl": "redis://redis7:6379", "workerCount": 1},
    {"redisUrl": "redis://valkey:6379", "workerCount": 1}
  ],
  "config": {
    "test": {
      "minClients": 1,
      "maxClients": 50,
      "stageIntervalMs": 1000,
      "requestDelayMs": 100,
      "keySize": 10,
      "valueSize": 100
    }
  }
}'

http_code=$(curl -s -o /tmp/job_resp.json -w "%{http_code}" \
  -X POST "$CONTROLLER_URL/job/start" \
  -H "Content-Type: application/json" \
  -d "$payload" || true)

if [ "$http_code" = "201" ] || [ "$http_code" = "409" ]; then
  log "job request completed with HTTP $http_code"
  cat /tmp/job_resp.json || true
  exit 0
fi

log "job request failed with HTTP $http_code"
cat /tmp/job_resp.json || true
exit 1
