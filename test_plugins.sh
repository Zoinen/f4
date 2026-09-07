#!/bin/bash
set -e

echo "1. Downloading Go dependencies..."
go mod tidy

echo "2. Building RPC Dummy Plugin..."
cd plugins/dummy_rpc
go build -o f4-rpc-dummy-plugin
cd ../..

TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT
export XDG_CONFIG_HOME="$TEST_ROOT/config"
LOG_PATH="$XDG_CONFIG_HOME/f4/Logs/debug.log"

echo "3. Running f4 in test mode (Output will go to $LOG_PATH)..."
VTUI_DEBUG=1 go run ./cmd/f4 -test-plugins

echo ""
echo "=== $LOG_PATH Output ==="
cat "$LOG_PATH"
