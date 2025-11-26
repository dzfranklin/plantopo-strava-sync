#!/bin/bash
# Deployment script for plantopo-strava-sync
# Run from project root: ./scripts/deploy.sh
# Builds for aarch64, uploads to app@pt0, and restarts service

set -euo pipefail

SSH_HOST="app@pt0"
APP_DIR="/home/app/plantopo-strava-sync"
SERVICE_NAME="plantopo-strava-sync"
BINARY_NAME="plantopo-strava-sync"

# Check if running from project root
if [[ ! -f "go.mod" ]]; then
    echo "Error: Must run from project root (go.mod not found)" >&2
    exit 1
fi

if ! grep -q "module plantopo-strava-sync" go.mod; then
    echo "Error: Unexpected go.mod content" >&2
    exit 1
fi

mkdir -p .deploy

# Test SSH connection
echo "Testing SSH connection..."
if ! ssh -o ConnectTimeout=10 -o BatchMode=yes "$SSH_HOST" "echo ok" &> /dev/null; then
    echo "Error: Cannot connect to $SSH_HOST" >&2
    exit 1
fi

# Build binary
echo "Building..."
if [[ -f "$BINARY_NAME" ]]; then
    rm "$BINARY_NAME"
fi

if ! CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ".deploy/$BINARY_NAME" -ldflags="-s -w" .; then
    echo "Error: Build failed" >&2
    exit 1
fi

# Upload binary
echo "Uploading binary..."
if ! scp -q ".deploy/$BINARY_NAME" "$SSH_HOST:$APP_DIR/$BINARY_NAME-next"; then
    echo "Error: Failed to upload binary" >&2
    exit 1
fi

ssh "$SSH_HOST" "chmod +x $APP_DIR/$BINARY_NAME-next && mv $APP_DIR/$BINARY_NAME-next $APP_DIR/$BINARY_NAME"

# Restart service
echo "Restarting service..."
if ! ssh "$SSH_HOST" "sudo systemctl restart $SERVICE_NAME"; then
    echo "Error: Failed to restart service" >&2
    exit 1
fi

# Wait and verify
sleep 5
if ! ssh "$SSH_HOST" "systemctl is-active --quiet $SERVICE_NAME"; then
    echo "Error: Service is not running" >&2
    ssh "$SSH_HOST" "journalctl -u $SERVICE_NAME -n 20"
    exit 1
fi

ssh "$SSH_HOST" "systemctl status $SERVICE_NAME"

echo "Deployment complete."
