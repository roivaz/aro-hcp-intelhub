#!/bin/bash

# cloud-provider-kind.sh - Manage cloud-provider-kind service
# Usage: ./cloud-provider-kind.sh {start|stop}

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
LOCALBIN="$PROJECT_DIR/bin"
CLOUD_PROVIDER_KIND="$LOCALBIN/cloud-provider-kind"
CLOUD_PROVIDER_KIND_VERSION="${CLOUD_PROVIDER_KIND_VERSION:-latest}"
CLOUD_PROVIDER_KIND_PID_FILE="${CLOUD_PROVIDER_KIND_PID_FILE:-/tmp/cloud-provider-kind.pid}"
CLOUD_PROVIDER_KIND_LOG_FILE="${CLOUD_PROVIDER_KIND_LOG_FILE:-$PROJECT_DIR/cloud-provider-kind.log}"

# Functions
ensure_binary() {
    if [[ ! -f "$CLOUD_PROVIDER_KIND" ]]; then
        echo "cloud-provider-kind binary not found. Installing..."
        mkdir -p "$LOCALBIN"
        echo "Downloading cloud-provider-kind@$CLOUD_PROVIDER_KIND_VERSION..."
        GOBIN="$LOCALBIN" go install "sigs.k8s.io/cloud-provider-kind@$CLOUD_PROVIDER_KIND_VERSION"
    fi
}

start_cloud_provider() {
    echo "Starting cloud-provider-kind in background..."
    
    if [[ -f "$CLOUD_PROVIDER_KIND_PID_FILE" ]] && kill -0 "$(cat "$CLOUD_PROVIDER_KIND_PID_FILE")" 2>/dev/null; then
        echo "cloud-provider-kind is already running (PID: $(cat "$CLOUD_PROVIDER_KIND_PID_FILE"))"
        return 0
    fi
    
    ensure_binary
    
    # Start cloud-provider-kind in background with output redirected to log file
    "$CLOUD_PROVIDER_KIND" > "$CLOUD_PROVIDER_KIND_LOG_FILE" 2>&1 &
    local pid=$!
    
    # Save PID to file
    echo "$pid" > "$CLOUD_PROVIDER_KIND_PID_FILE"
    
    echo "cloud-provider-kind started (PID: $pid)"
    echo "Logs are being written to: $CLOUD_PROVIDER_KIND_LOG_FILE"
}

stop_cloud_provider() {
    echo "Stopping cloud-provider-kind..."
    
    if [[ ! -f "$CLOUD_PROVIDER_KIND_PID_FILE" ]]; then
        echo "cloud-provider-kind PID file not found"
        return 0
    fi
    
    local pid
    pid=$(cat "$CLOUD_PROVIDER_KIND_PID_FILE")
    
    if kill -0 "$pid" 2>/dev/null; then
        kill "$pid"
        echo "cloud-provider-kind stopped (PID: $pid)"
    else
        echo "cloud-provider-kind process not found (PID: $pid)"
    fi
    
    rm -f "$CLOUD_PROVIDER_KIND_PID_FILE"
}

show_usage() {
    echo "Usage: $0 {start|stop}"
    echo ""
    echo "Commands:"
    echo "  start    Start cloud-provider-kind in background"
    echo "  stop     Stop cloud-provider-kind"
    exit 1
}

# Main
case "${1:-}" in
    start)
        start_cloud_provider
        ;;
    stop)
        stop_cloud_provider
        ;;
    *)
        show_usage
        ;;
esac
