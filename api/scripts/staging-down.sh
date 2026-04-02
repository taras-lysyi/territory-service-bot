#!/usr/bin/env bash
set -euo pipefail

# Stop staging bot
# Usage: ./scripts/staging-down.sh [--full]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
API_DIR="$(dirname "$SCRIPT_DIR")"
PID_FILE="$API_DIR/tmp/staging.pid"

cd "$API_DIR"

if [ ! -f "$PID_FILE" ]; then
    echo "No staging bot running (no PID file)"
    exit 0
fi

PID=$(cat "$PID_FILE")
if kill -0 "$PID" 2>/dev/null; then
    echo "Stopping staging bot (PID: $PID)..."
    kill "$PID"
    # Wait for clean exit
    for i in $(seq 1 5); do
        kill -0 "$PID" 2>/dev/null || break
        sleep 1
    done
    # Force kill if still running
    kill -9 "$PID" 2>/dev/null || true
    echo "Staging bot stopped"
else
    echo "Process $PID not running (stale PID file)"
fi

rm -f "$PID_FILE"

# Optional: stop Docker too
if [ "${1:-}" = "--full" ]; then
    echo "Stopping Docker containers..."
    docker compose down
fi
