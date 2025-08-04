# Build stage
FROM golang:1.24.2-bookworm AS build

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o redbench -a -ldflags="-s -w" -installsuffix cgo ./cmd/redbench

# Test stage
FROM build AS test

# Install test dependencies if needed
RUN go mod download

# Run tests
RUN go test -v ./...

# Run linting
RUN go vet ./...

# Production stage - Distroless for security & operational benefits
FROM gcr.io/distroless/static-debian12:nonroot AS production

# Set working directory
WORKDIR /app

# Copy binary and config to working directory
COPY --from=build /app/redbench /app/redbench
COPY --from=build /app/config.yaml /app/config.yaml

# Note: Health check via external tools not available in distroless
# Use Kubernetes liveness/readiness probes: GET http://container:8080/health

# Expose ports
EXPOSE 8080 8081

ENTRYPOINT ["/app/redbench"]