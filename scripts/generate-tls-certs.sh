#!/bin/bash

# Script to generate TLS certificates for Redis testing
# This script creates a CA, server certificate for Redis, and client certificates for mTLS

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CERTS_DIR="$PROJECT_ROOT/certs"

echo "Creating certificates in: $CERTS_DIR"
mkdir -p "$CERTS_DIR"

# Generate CA private key
echo "Generating CA private key..."
openssl genrsa -out "$CERTS_DIR/ca-key.pem" 4096

# Generate CA certificate
echo "Generating CA certificate..."
openssl req -new -x509 -days 365 -key "$CERTS_DIR/ca-key.pem" -out "$CERTS_DIR/ca.pem" \
    -subj "/C=US/ST=CA/L=San Francisco/O=RedBench Test/CN=Redis CA"

# Generate Redis server private key
echo "Generating Redis server private key..."
openssl genrsa -out "$CERTS_DIR/redis-key.pem" 4096

# Generate Redis server certificate signing request
echo "Generating Redis server CSR..."
openssl req -new -key "$CERTS_DIR/redis-key.pem" -out "$CERTS_DIR/redis.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=RedBench Test/CN=redis-tls"

# Create server certificate extensions
cat > "$CERTS_DIR/server_extensions.conf" << EOF
[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = redis-tls
DNS.2 = localhost
DNS.3 = redis.example.com
DNS.4 = cluster.example.com
DNS.5 = *.cluster.example.com
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

# Generate Redis server certificate
echo "Generating Redis server certificate..."
openssl x509 -req -days 365 -in "$CERTS_DIR/redis.csr" -CA "$CERTS_DIR/ca.pem" \
    -CAkey "$CERTS_DIR/ca-key.pem" -CAcreateserial -out "$CERTS_DIR/redis.pem" \
    -extensions v3_req -extfile "$CERTS_DIR/server_extensions.conf"

# Generate client private key (for mTLS)
echo "Generating client private key..."
openssl genrsa -out "$CERTS_DIR/client-key.pem" 4096

# Generate client certificate signing request
echo "Generating client CSR..."
openssl req -new -key "$CERTS_DIR/client-key.pem" -out "$CERTS_DIR/client.csr" \
    -subj "/C=US/ST=CA/L=San Francisco/O=RedBench Test/CN=redis-client"

# Create client certificate extensions
cat > "$CERTS_DIR/client_extensions.conf" << EOF
[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = clientAuth
EOF

# Generate client certificate
echo "Generating client certificate..."
openssl x509 -req -days 365 -in "$CERTS_DIR/client.csr" -CA "$CERTS_DIR/ca.pem" \
    -CAkey "$CERTS_DIR/ca-key.pem" -CAcreateserial -out "$CERTS_DIR/client.pem" \
    -extensions v3_req -extfile "$CERTS_DIR/client_extensions.conf"

# Set appropriate permissions
echo "Setting file permissions..."
chmod 600 "$CERTS_DIR"/*-key.pem
chmod 644 "$CERTS_DIR"/*.pem

# Clean up temporary files
rm -f "$CERTS_DIR"/*.csr "$CERTS_DIR"/*.conf

echo "✅ TLS certificates generated successfully in $CERTS_DIR/"
echo ""
echo "Files created:"
echo "  📜 ca.pem               - Certificate Authority (public)"
echo "  🔐 ca-key.pem           - CA private key"
echo "  📜 redis.pem            - Redis server certificate"
echo "  🔐 redis-key.pem        - Redis server private key"
echo "  📜 client.pem           - Client certificate (for mTLS)"
echo "  🔐 client-key.pem       - Client private key (for mTLS)"
echo ""
echo "Usage examples:"
echo "  # Basic TLS with CA verification:"
echo "  export REDIS_URL='rediss://localhost:6380'"
echo "  export REDIS_TLS_CA_FILE='$CERTS_DIR/ca.pem'"
echo "  export REDIS_TLS_INSECURE_SKIP_VERIFY='true'  # For localhost testing"
echo ""
echo "  # mTLS with client certificates:"
echo "  export REDIS_URL='rediss://localhost:6380'"
echo "  export REDIS_TLS_CA_FILE='$CERTS_DIR/ca.pem'"
echo "  export REDIS_TLS_CERT_FILE='$CERTS_DIR/client.pem'"
echo "  export REDIS_TLS_KEY_FILE='$CERTS_DIR/client-key.pem'"
echo "  export REDIS_TLS_INSECURE_SKIP_VERIFY='true'  # For localhost testing"
echo ""
echo "  # Test TLS connection:"
echo "  go run redbench/cmd/redbench/main.go"