# RedBench

Redis benchmarking tool that measures performance and provides Prometheus metrics.

## Project Structure

The project follows Clean Architecture principles:

- `cmd/redbench`: Main application entry point
- `internal/benchmark`: Core benchmark logic
- `internal/config`: Configuration handling
- `internal/metrics`: Prometheus metrics
- `internal/redis`: Redis client and operations
- `pkg/utils`: Shared utilities
- `test/integration`: Integration tests

## Configuration

Configuration is done via a YAML file (`config.yaml`) and environment variables:

- `REDIS_HOST` - Redis host (required if `REDIS_CLUSTER_ADDRESS` is not set)
- `REDIS_PORT` - Redis port (defaults to 6379)
- `REDIS_CLUSTER_ADDRESS` - Redis cluster address (required if `REDIS_HOST` is not set)

## Running Tests

### Unit Tests

Run the unit tests with:

```bash
# Set a test Redis host (required for tests)
REDIS_HOST=test-host go test -v ./...
```

### Integration Tests

Integration tests require a running Redis instance. You can start one with Docker:

```bash
docker run -d --name redis-test -p 6379:6379 redis:7
```

Then run the integration tests with:

```bash
REDIS_HOST=localhost go test -v -tags=integration ./test/integration
```

Don't forget to clean up after:

```bash
docker stop redis-test
docker rm redis-test
```

## CI/CD

This project uses GitHub Actions for continuous integration. The following checks are performed on every push and pull request:

1. **Unit Tests** - Run all unit tests
2. **Integration Tests** - Run integration tests with a Redis instance
3. **Linting** - Check code style and quality with golangci-lint

## Development

### Code Formatting and Linting

This project uses EditorConfig for consistent code formatting and pre-commit hooks to ensure code quality:

1. **EditorConfig**: Ensures consistent formatting across different editors. Make sure your editor supports EditorConfig or install the appropriate plugin.

2. **Pre-commit Hooks**: Install the pre-commit hooks to automatically check formatting before committing:

   ```bash
   ./scripts/install-hooks.sh
   ```

   These hooks will:
   - Run `go fmt` on all Go files
   - Run `golangci-lint` to check for code quality issues
   - Ensure files end with a newline
   - Check for trailing whitespace

3. **Manual Linting**: You can also run the linter manually:

   ```bash
   cd redbench && golangci-lint run
   ```

### Adding Tests

When adding new functionality, please also add corresponding tests:

1. Unit tests should be added in `*_test.go` files
2. Integration tests should be added in `test/integration` with the `//go:build integration` tag

### Test Coverage

To check test coverage:

```bash
go test -cover ./...
```

For a detailed coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```
