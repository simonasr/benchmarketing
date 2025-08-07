#!/bin/bash

# Test script for controller-worker coordination
# This script demonstrates how to run controller and workers

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REDBENCH_DIR="$(dirname "$SCRIPT_DIR")"
BINARY_PATH="$REDBENCH_DIR/redbench"

# Build the binary
echo "Building redbench..."
cd "$REDBENCH_DIR"
go build -o redbench cmd/redbench/main.go

# Function to cleanup background processes
cleanup() {
    echo "Cleaning up..."
    pkill -f "redbench --mode" || true
    wait 2>/dev/null || true
    echo "Cleanup complete"
}

# Set up cleanup on exit
trap cleanup EXIT

# Start controller
echo "Starting controller on port 8081..."
./redbench --mode=controller --port=8081 &
CONTROLLER_PID=$!

# Wait for controller to start
sleep 2

# Test controller health
echo "Testing controller health..."
curl -s http://localhost:8081/health | jq '.' || echo "Failed to get health status"

# Start workers
echo "Starting workers..."
./redbench --mode=worker --port=8080 --controller=http://localhost:8081 &
WORKER1_PID=$!

./redbench --mode=worker --port=8082 --controller=http://localhost:8081 &
WORKER2_PID=$!

# Wait for workers to register
sleep 3

# List workers
echo "Listing registered workers..."
curl -s http://localhost:8081/workers | jq '.' || echo "Failed to list workers"

# Test job creation (will fail without Redis, but demonstrates API)
echo "Testing job creation..."
curl -s -X POST http://localhost:8081/job/start \
  -H "Content-Type: application/json" \
  -d '{
    "targets": [
      {
        "redisUrl": "redis://localhost:6379",
        "workerCount": 1
      }
    ],
    "config": {
      "minClients": 1,
      "maxClients": 10,
      "stageIntervalMs": 1000,
      "requestDelayMs": 100,
      "keySize": 10,
      "valueSize": 100
    }
  }' | jq '.' || echo "Job creation failed (expected without Redis)"

# Check job status
echo "Checking job status..."
curl -s http://localhost:8081/job/status | jq '.' || echo "Failed to get job status"

echo "Demo completed. Check the logs above for results."
echo "Press Ctrl+C to stop all services."

# Keep running until Ctrl+C
wait
