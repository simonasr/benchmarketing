# Controller-Worker Architecture Usage

## Overview

RedBench now supports centralized control with a controller-worker architecture. This allows you to coordinate benchmark jobs across multiple worker instances.

## Architecture

```
┌─────────────────┐    ┌─────────────┐    ┌─────────────┐
│ redbench-worker │◄──►│  redbench-  │◄──►│ redbench-worker │
│    (Port 8080)  │    │ controller  │    │    (Port 8082)  │
└─────────────────┘    │ (Port 8081) │    └─────────────┘
                       └─────────────┘
                              │
                       ┌─────────────┐
                       │ redbench-worker │
                       │    (Port 8083)  │
                       └─────────────┘
```

## Usage

### 1. Start Controller

```bash
# Start controller on default port 8081
./redbench --mode=controller

# Start controller on custom port
./redbench --mode=controller --port=9000
```

### 2. Start Workers

```bash
# Start worker on default port 8080
./redbench --mode=worker --controller=http://localhost:8081

# Start multiple workers on different ports
./redbench --mode=worker --port=8080 --controller=http://localhost:8081
./redbench --mode=worker --port=8082 --controller=http://localhost:8081
./redbench --mode=worker --port=8083 --controller=http://localhost:8081

# Kubernetes deployment with explicit bind address
./redbench --mode=worker --port=8080 --controller=http://controller:8081 --bind-address=$POD_IP

# Local development with custom bind address
./redbench --mode=worker --port=8080 --controller=http://localhost:8081 --bind-address=192.168.1.100
```

#### Worker Address Registration

Workers automatically determine their registration address:
- **Auto-detect**: Uses hostname by default, or `localhost` for local development (when controller URL contains `localhost`)
- **Explicit**: Use `--bind-address` to override auto-detection (recommended for Kubernetes deployments)

### 3. Manage Jobs via Controller API

#### List Workers
```bash
curl http://localhost:8081/workers
```

#### Check Health
```bash
curl http://localhost:8081/health
```

#### Start Coordinated Job
```bash
curl -X POST http://localhost:8081/job/start \
  -H "Content-Type: application/json" \
  -d '{
    "targets": [
      {
        "redisUrl": "rediss://redis1.example.com:6380",
        "workerCount": 2
      },
      {
        "clusterUrl": "rediss://redis-cluster.example.com:6380",
        "workerCount": 2
      }
    ],
    "config": {
      "test": {
        "minClients": 10,
        "maxClients": 100,
        "stageIntervalMs": 1000,
        "requestDelayMs": 10,
        "keySize": 16,
        "valueSize": 1024
      },
      "redis": {
        "operationTimeoutMs": 250,
        "expiration": 45
      }
    }
  }'
```

- Use `redisUrl` for a single Redis instance.
- Use `clusterUrl` for Redis Cluster; both `redis://` and `rediss://` schemes are accepted. When a full URL is provided, the controller extracts the host:port for cluster clients while preserving TLS intent.
- Redis behavior overrides can be provided under `config.redis`:
  - `operationTimeoutMs`: per-operation timeout in milliseconds
  - `expiration`: TTL for generated keys in seconds
  - Note: YAML file uses `expirationS`, but JSON API uses `expiration`.

#### Stop Job
```bash
curl -X DELETE http://localhost:8081/job/stop
```

#### Check Job Status
```bash
curl http://localhost:8081/job/status
```

## API Reference

### Controller Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Controller health check |
| GET | `/workers` | List registered workers |
| POST | `/workers/register` | Register worker (automatic) |
| DELETE | `/workers/{id}` | Unregister worker |
| POST | `/job/start` | Start coordinated job |
| DELETE | `/job/stop` | Stop running job |
| GET | `/job/status` | Get job status |
| GET | `/metrics` | Prometheus metrics |

### Worker Endpoints (unchanged)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/status` | Worker benchmark status |
| POST | `/start` | Start benchmark on worker |
| DELETE | `/stop` | Stop benchmark on worker |
| GET | `/metrics` | Worker Prometheus metrics |

## Example Job Request

```json
{
  "targets": [
    {
      "redisUrl": "rediss://redis-cluster-1.example.com:6380",
      "workerCount": 3
    },
    {
      "clusterUrl": "rediss://redis-cluster-2.example.com:6380",
      "workerCount": 2
    }
    {
      "clusterUrl": "rediss://redis-cluster-2.example.com:6380",
      "workerCount": 2
    }
  ],
  "config": {
    "test": {
      "minClients": 1,
      "maxClients": 200,
      "stageIntervalMs": 2000,
      "requestDelayMs": 50,
      "keySize": 20,
      "valueSize": 2048
    },
    "redis": {
      "operationTimeoutMs": 200,
      "expiration": 30
    }
  }
}
```

This will:
- Assign 3 workers to benchmark `redis-cluster-1`
- Assign 2 workers to benchmark `redis-cluster-2`
- Ramp up from 1 to 200 clients over time with 2-second intervals
- Use 20-byte keys and 2KB values with 50ms request delays

## Demo Script

Use the provided demo script to test the controller-worker setup:

```bash
./scripts/test-controller-worker.sh
```

## Legacy Modes (unchanged)

```bash
# Traditional CLI mode
./redbench

# Single service mode (uses port 8080 by default)
./redbench --service
```

## Notes

- **Port allocation**: Controller uses 8081, workers use 8080 (and other ports)
- Workers automatically register/unregister with the controller
- Jobs are stateless - controller doesn't persist state across restarts
- Controller coordinates job distribution but workers handle actual benchmarking
- All existing benchmarking logic and configuration remains unchanged
- Configuration uses actual benchmark parameters: `minClients`, `maxClients`, `stageIntervalMs`, `requestDelayMs`, `keySize`, `valueSize`
