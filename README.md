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

## Acknowledgments

Inspired by [Anton Putra's Tutorials](https://github.com/antonputra/tutorials/tree/main)

---
_Note: This is a benchmarking playground for educational and testing purposes._