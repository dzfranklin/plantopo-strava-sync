#!/bin/bash
# Development proxy script
#
# This creates an SSH reverse tunnel that forwards port 4101 on the server (pt0)
# to port 4101 on your local development machine. This allows Caddy on the server
# to forward requests to your local development instance.
#
# Usage: ./dev-proxy.sh

set -e

SERVER="app@pt0"
REMOTE_PORT=4101
LOCAL_PORT=4101

echo "🔌 Setting up reverse SSH tunnel..."
echo "   Remote: ${SERVER}:${REMOTE_PORT}"
echo "   Local:  localhost:${LOCAL_PORT}"
echo ""
echo "Press Ctrl+C to disconnect"
echo ""

# -R: Reverse tunnel (remote port forwards to local)
# -N: Don't execute remote command (just forward ports)
# -T: Disable pseudo-terminal allocation
ssh -R ${REMOTE_PORT}:localhost:${LOCAL_PORT} -N -T ${SERVER}
