# Service Mode API Demo

This document demonstrates how to use the new service mode functionality.

## Starting the Service

1. **Start in service mode:**
```bash
REDIS_HOST=localhost API_PORT=8080 ./redbench --service
```

2. **Check status (should be idle initially):**
```bash
curl -X GET http://localhost:8080/status | jq
```

Expected response:
```json
{
  "status": "idle",
  "configuration": null,
  "redisTarget": null,
  "startTime": null,
  "endTime": null,
  "errorMessage": ""
}
```

## Starting a Benchmark

3. **Start a benchmark with default configuration:**
```bash
curl -X POST http://localhost:8080/start | jq
```

4. **Start a benchmark with custom parameters:**
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "test": {
      "minClients": 5,
      "maxClients": 50,
      "stageIntervalS": 2,
      "keySize": 15,
      "valueSize": 20
    }
  }' | jq
```

5. **Start a benchmark with a different Redis target:**
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "host": "redis-cluster.example.com",
      "port": "6380"
    },
    "test": {
      "maxClients": 100
    }
  }' | jq
```

6. **Start a benchmark with Redis URL (including TLS):**
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "url": "rediss://secure-redis.example.com:6380"
    }
  }' | jq
```

7. **Start a benchmark with Redis cluster:**
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "clusterUrl": "redis://cluster.example.com:6379"
    }
  }' | jq
```

8. **Start a benchmark with custom TLS configuration:**
```bash
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{
    "redis": {
      "host": "secure-redis.example.com",
      "tls": {
        "enabled": true,
        "caFile": "/path/to/ca.pem",
        "serverName": "secure-redis.example.com"
      }
    }
  }' | jq
```

Expected response:
```json
{
  "status": "running",
  "configuration": {...},
  "redisTarget": {
    "host": "secure-redis.example.com",
    "port": "6380",
    "targetLabel": "secure-redis.example.com:6380",
    "tls": {
      "enabled": true,
      "caFile": "/path/to/ca.pem",
      "serverName": "secure-redis.example.com"
    }
  },
  "startTime": "2025-01-05T13:04:02Z",
  "endTime": null,
  "errorMessage": ""
}
```

## Monitoring Progress

5. **Check status during execution:**
```bash
curl -X GET http://localhost:8080/status | jq
```

6. **Monitor Prometheus metrics:**
```bash
curl http://localhost:8081/metrics | grep redbench
```

## Stopping a Benchmark

7. **Stop a running benchmark:**
```bash
curl -X DELETE http://localhost:8080/stop | jq
```

Expected response:
```json
{
  "status": "stopped",
  "configuration": {...},
  "startTime": "2025-01-05T13:04:02Z",
  "endTime": "2025-01-05T13:04:10Z",
  "errorMessage": ""
}
```

## Error Handling

8. **Try to start when already running (should return 409):**
```bash
curl -X POST http://localhost:8080/start
# Response: HTTP 409 Conflict
```

9. **Try to stop when not running (should return 409):**
```bash
curl -X DELETE http://localhost:8080/stop
# Response: HTTP 409 Conflict
```

## Redis Configuration Options

The service supports flexible Redis target configuration via the API request:

### Connection Methods

1. **Host/Port** (traditional):
```json
{
  "redis": {
    "host": "redis.example.com",
    "port": "6379"
  }
}
```

2. **Redis URL** (supports redis:// and rediss://):
```json
{
  "redis": {
    "url": "rediss://secure-redis.example.com:6380"
  }
}
```

3. **Cluster URL**:
```json
{
  "redis": {
    "clusterUrl": "redis://cluster.example.com:6379"
  }
}
```

### TLS Configuration

You can specify TLS settings per benchmark:

```json
{
  "redis": {
    "host": "redis.example.com",
    "tls": {
      "enabled": true,
      "caFile": "/path/to/ca.pem",
      "certFile": "/path/to/client.pem",
      "keyFile": "/path/to/client-key.pem",
      "insecureSkipVerify": false,
      "serverName": "redis.example.com"
    }
  }
}
```

### Fallback Behavior

- If no `redis` configuration is provided in the API request, the service uses the default Redis connection from environment variables
- Each benchmark can target a different Redis instance
- Metrics are labeled with the target Redis instance for proper separation

## Configuration Priority

The configuration follows this priority order:
1. **API Request Body** (highest) - JSON parameters in POST request
2. **Environment Variables** (medium) - `TEST_*` variables  
3. **config.yaml** (lowest) - Default configuration file

Example with environment override:
```bash
TEST_MIN_CLIENTS=10 TEST_MAX_CLIENTS=100 ./redbench --service
```

## Backwards Compatibility

The original CLI mode still works exactly as before:
```bash
REDIS_HOST=localhost ./redbench
```

This runs a one-shot benchmark and exits, just like before.