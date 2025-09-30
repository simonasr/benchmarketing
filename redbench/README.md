# RedBench

Redis benchmarking tool that measures performance and provides Prometheus metrics.

## Features

- **CLI Mode**: Traditional one-shot benchmarking
- **Service Mode**: HTTP API for remote control and multiple benchmarks
- **Controller/Worker Mode**: Centralized orchestration across workers with optional UI
- **Prometheus Metrics**: Real-time performance monitoring

- **Cluster Support**: Redis cluster and single-instance modes
- **Flexible Configuration**: YAML files, environment variables, and API overrides

## Quick Start

### CLI Mode (One-shot benchmark)

```bash
# Single Redis instance
REDIS_URL=redis://localhost:6379 ./redbench

# Redis cluster
REDIS_CLUSTER_URL=redis://cluster.example.com:6379 ./redbench
```

### Service Mode (HTTP API)

```bash
# Start the service (Redis config optional in service mode)
API_PORT=8080 ./redbench -service

# Start a benchmark via API
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "url": "redis://localhost:6379"
    }
  }' | jq

# Check status
curl http://localhost:8080/status | jq

# Stop benchmark
curl -X DELETE http://localhost:8080/stop | jq
```

### Controller/Worker Mode (Docker Compose)

```bash
# 1) Start the stack (controller, workers, Redis, Prometheus, Grafana)
docker compose -f compose-example.yaml up --build

# 2) Start a coordinated job via the controller (port 8081)
curl -X POST http://localhost:8081/job/start \
  -H "Content-Type: application/json" \
  -d '{
    "targets": [
      {"redisUrl": "redis://redis8:6379", "workerCount": 1},
      {"redisUrl": "redis://redis7:6379", "workerCount": 1},
      {"redisUrl": "redis://valkey:6379", "workerCount": 1}
    ],
    "config": {
      "test": { "minClients": 1, "maxClients": 50, "stageIntervalMs": 1000, "requestDelayMs": 100, "keySize": 10, "valueSize": 100 }
    }
  }'

# 3) Inspect controller and workers
curl http://localhost:8081/health
curl http://localhost:8081/workers
curl http://localhost:8081/job/status

# UI (served by controller)
# http://localhost:8081/ui/

# Tip: optional autostart profile to kick off a job at boot
# docker compose -f compose-example.yaml --profile autostart up --build
```

## Project Structure

The project follows Clean Architecture principles:

- `cmd/redbench`: Main application entry point
- `internal/benchmark`: Core benchmark logic
- `internal/config`: Configuration handling
- `internal/metrics`: Prometheus metrics
- `internal/redis`: Redis client and operations
- `internal/service`: HTTP API service
- `pkg/utils`: Shared utilities
- `test/integration`: Integration tests

## Configuration

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `REDIS_URL` | Single Redis instance URL (`redis://`) | Yes (if `REDIS_CLUSTER_URL` not set) |
| `REDIS_CLUSTER_URL` | Redis cluster URL (`redis://`) | Yes (if `REDIS_URL` not set) |
| `API_PORT` | HTTP API port for service mode | No (default: 8080) |
| `TEST_*` | Override test parameters (e.g., `TEST_MAX_CLIENTS=100`) | No |

### Configuration Priority

1. **API Request Body** (highest) - JSON parameters in service mode
2. **Environment Variables** (medium) - Runtime overrides
3. **config.yaml** (lowest) - Default configuration file

## Service Mode API

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/status` | Get current benchmark status |
| `POST` | `/start` | Start a new benchmark |
| `DELETE` | `/stop` | Stop running benchmark |

### Starting Benchmarks

All `/start` requests **must** include Redis configuration. Here are examples:

#### Basic Redis Connection
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "url": "redis://localhost:6379"
    }
  }'
```

#### Custom Test Parameters
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "url": "redis://localhost:6379"
    },
    "test": {
      "minClients": 5,
      "maxClients": 50,
      "stageIntervalMs": 2000,
      "keySize": 15,
      "valueSize": 20
    }
  }'
```

#### Redis Cluster
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "clusterUrl": "redis://cluster.example.com:6379"
    }
  }'
```

### Response Examples

#### Status Response
```json
{
  "status": "running",
  "configuration": {
    "Debug": false,
    "Redis": { ... },
    "Test": { ... }
  },
  "redisTarget": {
    "url": "redis://localhost:6379",
    "targetLabel": "redis://localhost:6379",
    "connectTimeoutSeconds": 10
  },
  "startTime": "2025-01-05T13:04:02Z",
  "endTime": null,
  "errorMessage": ""
}
```

#### Error Response (Missing Redis Config)
```bash
# HTTP 400 Bad Request
Redis connection requires either URL or ClusterURL to be specified
```

### Error Handling

| Status Code | Description |
|-------------|-------------|
| `200` | Success |
| `201` | Benchmark started |
| `400` | Bad request (invalid config, missing Redis target) |
| `409` | Conflict (benchmark already running/not running) |
| `500` | Internal server error |

## Monitoring

### Prometheus Metrics

Metrics are exposed on each running server under `/metrics`:

- In CLI and service modes, the default port is 8080 (configurable via `API_PORT`).
- In controller/worker mode (Compose), the controller exposes `http://localhost:8081/metrics`; workers are scraped inside the Compose network by Prometheus.

```bash
# Example (service/CLI mode):
curl http://localhost:8080/metrics | grep redbench

# Example (controller mode):
curl http://localhost:8081/metrics | grep redbench
```

Key metrics:
- `redbench_operations_total` - Total operations performed
- `redbench_operation_duration_seconds` - Operation latency
- `redbench_errors_total` - Error count
- `redbench_active_clients` - Current client count

## Development

### Running Tests

#### Unit Tests
```bash
# Set a test Redis URL (required for tests)
REDIS_URL=redis://test-host:6379 go test -v ./...
```

#### Integration Tests
```bash
# Start Redis for testing
docker run -d --name redis-test -p 6379:6379 redis:7

# Run integration tests
REDIS_URL=redis://localhost:6379 go test -v -tags=integration ./test/integration

# Cleanup
docker stop redis-test && docker rm redis-test
```

### Code Quality

#### Pre-commit Hooks
```bash
# Install development tools and hooks
./scripts/install-hooks.sh
```

The hooks will automatically:
- Run `go fmt` on all Go files
- Run `golangci-lint` for code quality
- Check for trailing whitespace
- Ensure files end with newlines

#### Manual Linting
```bash
golangci-lint run
```

#### Test Coverage
```bash
# Basic coverage
go test -cover ./...

# Detailed HTML report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Adding Tests

1. **Unit tests**: Add `*_test.go` files alongside your code
2. **Integration tests**: Add files in `test/integration/` with `//go:build integration` tag

## Examples

### Docker Compose Setup

See `compose-example.yaml` for a complete controller/worker setup with Redis, Prometheus, and Grafana.

### CI/CD

GitHub Actions automatically runs:
1. Unit tests
2. Integration tests
3. Linting checks

## Troubleshooting

### Common Issues

1. **"Redis connection requires either URL or ClusterURL"**
   - Ensure you provide Redis configuration in CLI mode via environment variables
   - In service mode, include `redis` configuration in your POST request

2. **Port conflicts**
   - Change API/metrics port: `API_PORT=9090 ./redbench -service`
   - Both API and metrics now use the same unified port (8080 by default)

### Debug Mode

Enable debug logging by setting `debug: true` in `config.yaml` or running with debug environment variables.
