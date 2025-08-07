# RedBench

Redis benchmarking tool that measures performance and provides Prometheus metrics.

## Features

- **CLI Mode**: Traditional one-shot benchmarking
- **Service Mode**: HTTP API for remote control and multiple benchmarks
- **Prometheus Metrics**: Real-time performance monitoring
- **TLS Support**: Secure connections with certificate validation
- **Cluster Support**: Redis cluster and single-instance modes
- **Flexible Configuration**: YAML files, environment variables, and API overrides

## Quick Start

### CLI Mode (One-shot benchmark)

```bash
# Single Redis instance
REDIS_URL=redis://localhost:6379 ./redbench

# Redis cluster
REDIS_CLUSTER_URL=redis://cluster.example.com:6379 ./redbench

# TLS connection
REDIS_URL=rediss://secure-redis.example.com:6379 ./redbench
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
| `REDIS_URL` | Single Redis instance URL (`redis://` or `rediss://`) | Yes (if `REDIS_CLUSTER_URL` not set) |
| `REDIS_CLUSTER_URL` | Redis cluster URL (`redis://` or `rediss://`) | Yes (if `REDIS_URL` not set) |
| `API_PORT` | HTTP API port for service mode | No (default: 8080) |
| `TEST_*` | Override test parameters (e.g., `TEST_MAX_CLIENTS=100`) | No |
| `REDIS_TLS_*` | TLS configuration (see TLS section) | No |

### Configuration Priority

1. **API Request Body** (highest) - JSON parameters in service mode
2. **Environment Variables** (medium) - Runtime overrides
3. **config.yaml** (lowest) - Default configuration file

### TLS Configuration

```bash
# TLS Environment Variables
export REDIS_TLS_CA_FILE="/path/to/ca.pem"
export REDIS_TLS_CERT_FILE="/path/to/client.pem"      # mTLS only
export REDIS_TLS_KEY_FILE="/path/to/client-key.pem"   # mTLS only
export REDIS_TLS_SERVER_NAME="redis.example.com"
export REDIS_TLS_INSECURE_SKIP_VERIFY="false"
```

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

#### TLS Connection
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "url": "rediss://secure-redis.example.com:6379",
      "tls": {
        "caFile": "/path/to/ca.pem",
        "serverName": "secure-redis.example.com"
      }
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

#### mTLS with Client Certificates
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "url": "rediss://secure-redis.example.com:6379",
      "tls": {
        "caFile": "/path/to/ca.pem",
        "certFile": "/path/to/client.pem",
        "keyFile": "/path/to/client-key.pem",
        "serverName": "secure-redis.example.com"
      }
    }
  }'
```

### Response Examples

#### Status Response
```json
{
  "status": "running",
  "configuration": {
    "MetricsPort": 8081,
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

Metrics are available at `http://localhost:8081/metrics`:

```bash
# View all redbench metrics
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

See `compose-example.yaml` for a complete setup with Redis, Grafana, and Prometheus.

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

2. **TLS connection failures**
   - Verify CA file path and certificate validity
   - Check server name matches certificate
   - Use `insecureSkipVerify: true` only for testing

3. **Service mode port conflicts**
   - Change API port: `API_PORT=9090 ./redbench -service`
   - Change metrics port in `config.yaml`

### Debug Mode

Enable debug logging by setting `debug: true` in `config.yaml` or running with debug environment variables.
