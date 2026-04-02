#!/usr/bin/env bash
set -euo pipefail

# Start staging bot in background with health check
# Usage: ./scripts/staging-up.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
API_DIR="$(dirname "$SCRIPT_DIR")"
PID_FILE="$API_DIR/tmp/staging.pid"
LOG_FILE="$API_DIR/tmp/staging.log"
HEALTH_URL="http://localhost:8080/health"
MAX_WAIT=30

cd "$API_DIR"

# Check if already running
if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
    echo "Staging bot already running (PID: $(cat "$PID_FILE"))"
    exit 0
fi

# Check if port 8080 is already in use
if lsof -ti:8080 >/dev/null 2>&1; then
    echo "ERROR: Port 8080 already in use"
    lsof -i:8080
    echo "Kill the process or run: make staging-down"
    exit 1
fi

# Ensure tmp dir exists
mkdir -p tmp

# Start PostgreSQL if not running
if ! docker compose ps postgres 2>/dev/null | grep -q "running"; then
    echo "Starting PostgreSQL..."
    docker compose up -d
    sleep 3
fi

# Seed database
echo "Seeding database..."
go run scripts/seed.go 2>&1 | tail -1

# Build binary (single PID, no go run child process issues)
echo "Building staging bot..."
go build -o tmp/staging-bot cmd/main.go

# Source staging env and start in background
echo "Starting staging bot..."
set -a
source .env.staging
set +a
tmp/staging-bot > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

# Poll health endpoint
echo -n "Waiting for bot to be ready"
for i in $(seq 1 $MAX_WAIT); do
    if curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
        echo ""
        echo "Staging bot ready (PID: $(cat "$PID_FILE"), health: $HEALTH_URL)"
        exit 0
    fi
    echo -n "."
    sleep 1
done

echo ""
echo "ERROR: Bot failed to start within ${MAX_WAIT}s"
echo "Check logs: $LOG_FILE"
# Cleanup on failure
kill "$(cat "$PID_FILE")" 2>/dev/null || true
rm -f "$PID_FILE"
exit 1
