# Benchmarketing

A playground for exploring and comparing performance benchmarks across different technologies and implementations.

## Getting Started

```bash
# Add installation steps here
git clone https://github.com/yourusername/benchmarketing.git
cd benchmarketing
```

## Usage

### Starting the Services

```bash
# Start all services in detached mode
docker compose -f compose-example.yaml up -d

# To view logs
docker compose -f compose-example.yaml logs -f

# To stop all services
docker compose -f compose-example.yaml down
```

### Accessing Monitoring Tools

1. **Grafana Dashboard**
   - URL: http://localhost:3000
   - Credentials: admin/grafana
   - Default dashboards are available under the "Dashboards" menu

2. **Prometheus**
   - URL: http://localhost:9090
   - Used for metrics collection

### Running Benchmarks

"redbench" instances will automatically start running benchmarks against Redis and Valkey services.

### Troubleshooting

```bash
# Check service status
docker compose -f compose-example.yaml ps

# Restart specific service
docker compose -f compose-example.yaml restart [service-name]

# View service logs
docker compose -f compose-example.yaml logs [service-name]
```

## Redis TLS Configuration

RedBench supports secure Redis connections using TLS. Configure TLS using environment variables or URLs.

### Basic Setup

```bash
# Using Redis URL (recommended)
export REDIS_URL="rediss://redis.example.com:6380"
export REDIS_TLS_CA_FILE="/path/to/ca.pem"

# Using traditional host/port
export REDIS_HOST="redis.example.com"
export REDIS_TLS_ENABLED="true"
export REDIS_TLS_CA_FILE="/path/to/ca.pem"

# Redis cluster
export REDIS_CLUSTER_URL="rediss://cluster.example.com:6380"
export REDIS_TLS_CA_FILE="/path/to/ca.pem"
```

### Key Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `REDIS_URL` | Redis URL (rediss:// for TLS) | `rediss://redis.example.com:6380` |
| `REDIS_CLUSTER_URL` | Redis cluster URL | `rediss://cluster.example.com:6380` |
| `REDIS_TLS_CA_FILE` | Path to CA certificate | `/certs/ca.pem` |
| `REDIS_TLS_CERT_FILE` | Client certificate (mTLS) | `/certs/client.pem` |
| `REDIS_TLS_KEY_FILE` | Client private key (mTLS) | `/certs/client-key.pem` |
| `REDIS_TLS_INSECURE_SKIP_VERIFY` | Skip verification (testing) | `true` |

### Testing with Docker

```bash
# Generate test certificates
./scripts/generate-tls-certs.sh

# Test TLS connection (insecure for testing)
export REDIS_URL="rediss://localhost:6380"
export REDIS_TLS_INSECURE_SKIP_VERIFY="true"
go run redbench/cmd/redbench/main.go
```

## Acknowledgments

Inspired by [Anton Putra's Tutorials](https://github.com/antonputra/tutorials/tree/main)

---
_Note: This is a benchmarking playground for educational and testing purposes._