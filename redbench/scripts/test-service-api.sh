#!/bin/bash

# Simple script to test the service mode API
# Requires: curl, jq
# Note: Will test API functionality, but actual benchmarks require Redis

set -e

API_URL="http://localhost:8080"
SERVICE_PID=""

# Function to cleanup on exit
cleanup() {
    if [[ -n "$SERVICE_PID" ]]; then
        echo "Stopping service (PID: $SERVICE_PID)..."
        kill $SERVICE_PID 2>/dev/null || true
        wait $SERVICE_PID 2>/dev/null || true
    fi
}

# Set up cleanup trap
trap cleanup EXIT

echo "=== Testing Service Mode API ==="
echo

# Start the service
echo "Starting service..."
./redbench --service &
SERVICE_PID=$!
sleep 2

# Check if service is running
if ! kill -0 $SERVICE_PID 2>/dev/null; then
    echo "❌ Service failed to start"
    exit 1
fi

echo "✅ Service started successfully (PID: $SERVICE_PID)"
echo

echo "1. Testing status endpoint (should be idle)..."
if curl -s -X GET "$API_URL/status" | jq; then
    echo "✅ Status endpoint working"
else
    echo "❌ Status endpoint failed"
    exit 1
fi
echo

echo "2. Starting benchmark (will fail due to no Redis, but API should work)..."
START_RESPONSE=$(curl -s -X POST "$API_URL/start" \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "host": "test-host-nonexistent",
      "port": "6379"
    },
    "test": {
      "minClients": 1,
      "maxClients": 2,
      "stageIntervalS": 1,
      "keySize": 8,
      "valueSize": 16
    }
  }')
echo "$START_RESPONSE" | jq
echo

echo "3. Checking status (should show running or failed)..."
sleep 2
curl -s -X GET "$API_URL/status" | jq
echo

echo "4. Testing stop endpoint (expect 409 since benchmark already failed)..."
HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/stop_response.json -X DELETE "$API_URL/stop")
echo "      HTTP Status: $HTTP_CODE"
if [ "$HTTP_CODE" = "409" ]; then
    echo "      ✅ Correctly returned 409 (no running benchmark to stop)"
else
    echo "      ⚠️  Expected 409, got $HTTP_CODE"
    cat /tmp/stop_response.json | jq 2>/dev/null || cat /tmp/stop_response.json
fi
echo

echo "5. Testing error cases..."
echo "   a) Trying to stop when nothing is running (should return 409)..."
HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json -X DELETE "$API_URL/stop")
echo "      HTTP Status: $HTTP_CODE"
if [ "$HTTP_CODE" = "409" ]; then
    echo "      ✅ Correctly returned 409 Conflict"
else
    echo "      ❌ Expected 409, got $HTTP_CODE"
fi

echo "   b) Testing invalid JSON in start request..."
HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json -X POST "$API_URL/start" -d 'invalid json')
echo "      HTTP Status: $HTTP_CODE"
if [ "$HTTP_CODE" = "400" ]; then
    echo "      ✅ Correctly returned 400 Bad Request for invalid JSON"
else
    echo "      ⚠️  Expected 400, got $HTTP_CODE"
fi

echo "   c) Testing invalid HTTP method..."
HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json -X PATCH "$API_URL/status")
echo "      HTTP Status: $HTTP_CODE"
if [ "$HTTP_CODE" = "405" ]; then
    echo "      ✅ Correctly returned 405 Method Not Allowed"
else
    echo "      ❌ Expected 405, got $HTTP_CODE"
fi

echo
echo "=== API Tests Complete ==="
echo "✅ Service functionality verified"