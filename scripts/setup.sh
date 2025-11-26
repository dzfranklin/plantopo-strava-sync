#!/bin/bash
# Setup script for plantopo-strava-sync
# Run once on the server as root: sudo bash setup.sh
# Creates app user, directories, systemd service, and sudoers config

set -euo pipefail

APP_USER="app"
APP_DIR="/home/app/plantopo-strava-sync"
SERVICE_NAME="plantopo-strava-sync"

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    echo "Error: This script must be run as root" >&2
    exit 1
fi

# Create app user if it doesn't exist
if id "$APP_USER" &>/dev/null; then
    echo "User '$APP_USER' already exists"
else
    echo "Creating user '$APP_USER'..."
    useradd -r -m -s /bin/bash "$APP_USER"
fi

# Create application directory
if [[ ! -d "$APP_DIR" ]]; then
    mkdir -p "$APP_DIR"
fi
chown -R "$APP_USER:$APP_USER" "$APP_DIR"
chmod 755 "$APP_DIR"

# Create .env template
env_file="$APP_DIR/.env"
if [[ -f "$env_file" ]]; then
    echo ".env file already exists, skipping"
else
    echo "Creating .env template..."
    cat > "$env_file" <<'EOF'
# plantopo-strava-sync configuration
DOMAIN=connect-with-strava.plantopo.com
HOST=127.0.0.1
PORT=4101
DATABASE_PATH=/home/app/plantopo-strava-sync/data.db

# Strava API configuration - PRIMARY CLIENT (REQUIRED)
STRAVA_PRIMARY_CLIENT_ID=
STRAVA_PRIMARY_CLIENT_SECRET=
STRAVA_PRIMARY_VERIFY_TOKEN=

# Strava API configuration - SECONDARY CLIENT (OPTIONAL)
#STRAVA_SECONDARY_CLIENT_ID=
#STRAVA_SECONDARY_CLIENT_SECRET=
#STRAVA_SECONDARY_VERIFY_TOKEN=

# Internal API authentication (REQUIRED)
INTERNAL_API_KEY=

# Logging and metrics
LOG_LEVEL=info
METRICS_ENABLED=true
METRICS_HOST=127.0.0.1
METRICS_PORT=4102
EOF

    chmod 600 "$env_file"
    chown "$APP_USER:$APP_USER" "$env_file"
    echo "Created .env template at $env_file"
fi

# Create CLI wrapper script
cli_script="$APP_DIR/cli.sh"
echo "Creating CLI wrapper..."
cat > "$cli_script" <<'EOF'
#!/bin/bash
# Usage: ./cli.sh [args...]
# Sources .env and executes plantopo-strava-sync with arguments

set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"
BINARY="$SCRIPT_DIR/plantopo-strava-sync"

if [[ ! -f "$ENV_FILE" ]]; then
    echo "Error: .env file not found at $ENV_FILE" >&2
    exit 1
fi

if [[ ! -f "$BINARY" ]]; then
    echo "Error: Binary not found at $BINARY" >&2
    exit 1
fi

set -a
source "$ENV_FILE"
set +a
exec "$BINARY" "$@"
EOF

chmod 755 "$cli_script"
chown "$APP_USER:$APP_USER" "$cli_script"

# Create systemd service file
service_file="/etc/systemd/system/${SERVICE_NAME}.service"
if [[ -f "$service_file" ]]; then
    cp "$service_file" "${service_file}.backup.$(date +%s)"
fi

echo "Creating systemd service..."
cat > "$service_file" <<EOF
[Unit]
Description=Plantopo Strava Sync Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$APP_USER
Group=$APP_USER
WorkingDirectory=$APP_DIR
EnvironmentFile=$APP_DIR/.env
ExecStart=$APP_DIR/$SERVICE_NAME
Restart=on-failure
RestartSec=10s
TimeoutStopSec=15s

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$APP_DIR

[Install]
WantedBy=multi-user.target
EOF

chmod 644 "$service_file"

# Configure passwordless sudo
sudoers_file="/etc/sudoers.d/$SERVICE_NAME"
if [[ ! -f "$sudoers_file" ]]; then
    echo "Configuring sudo..."
    temp_file=$(mktemp)
    cat > "$temp_file" <<EOF
$APP_USER ALL=(ALL) NOPASSWD: /bin/systemctl restart $SERVICE_NAME, /bin/systemctl status $SERVICE_NAME, /bin/systemctl stop $SERVICE_NAME, /bin/systemctl start $SERVICE_NAME
EOF

    if visudo -c -f "$temp_file" >/dev/null 2>&1; then
        mv "$temp_file" "$sudoers_file"
        chmod 440 "$sudoers_file"
    else
        echo "Error: Sudoers syntax validation failed" >&2
        rm "$temp_file"
        exit 1
    fi
fi

# Reload systemd and enable service
echo "Enabling service..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo ""
echo "Setup complete."
echo ""
echo "Next steps:"
echo "1. Edit $APP_DIR/.env and fill in required secrets"
echo "2. Run ./scripts/deploy.sh from your development machine"
echo "3. Check status: sudo systemctl status $SERVICE_NAME"
echo "4. View logs: journalctl -u $SERVICE_NAME -f"
