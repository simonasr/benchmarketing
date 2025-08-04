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

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/redbench", "-service"] || exit 1

# Expose ports
EXPOSE 8080 8081

ENTRYPOINT ["/app/redbench"]