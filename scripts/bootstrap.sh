#!/bin/bash
set -euo pipefail

echo "=== CAAS Development Environment Bootstrap ==="
echo ""

# Check required tools
check_tool() {
    if ! command -v "$1" &>/dev/null; then
        echo "ERROR: $1 is not installed. $2"
        exit 1
    fi
    echo "  ✓ $1 found: $($1 --version 2>&1 | head -1)"
}

echo "Checking prerequisites..."
check_tool docker "Install Docker Desktop from https://docker.com"
check_tool go "Install Go from https://go.dev/dl/"
check_tool buf "Install buf: https://buf.build/docs/installation"
check_tool node "Install Node.js from https://nodejs.org"
echo ""

# Start infrastructure
echo "Starting infrastructure services..."
docker compose up -d
echo ""

# Wait for health
echo "Waiting for services to be healthy..."
sleep 10

# Check services
echo "Checking service health..."
docker compose ps
echo ""

# Generate proto code
echo "Generating protobuf code..."
buf lint && buf generate
echo ""

# Load SpiceDB schema
echo "Loading SpiceDB authorization schema..."
if command -v zed &>/dev/null; then
    zed schema write infra/spicedb-schema.zed --insecure --endpoint localhost:50051 --token caas_dev_key
else
    echo "  (zed CLI not installed — load schema manually or via API)"
fi
echo ""

echo "=== Bootstrap Complete ==="
echo ""
echo "Services running at:"
echo "  PostgreSQL:       localhost:5432    (user: caas / pass: caas_dev)"
echo "  Redis:            localhost:6379"
echo "  Neo4j Browser:    http://localhost:7474  (neo4j / caas_dev_password)"
echo "  SpiceDB gRPC:     localhost:50051   (key: caas_dev_key)"
echo "  SpiceDB HTTP:     http://localhost:8443"
echo "  Redpanda Kafka:   localhost:19092"
echo "  Redpanda Console: http://localhost:8080"
echo ""
echo "Next steps:"
echo "  make proto   — Regenerate proto code"
echo "  make build   — Build Go services"
echo "  make test    — Run tests"
echo "  make seed    — Load demo data"
