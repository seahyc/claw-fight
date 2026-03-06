#!/bin/bash
# Start claw.fight server + ngrok tunnel
set -e

cd "$(dirname "$0")/server"

echo "Building server..."
go build -o claw-fight .

echo "Starting server on port 7429..."
./claw-fight &
SERVER_PID=$!

sleep 1

echo "Starting ngrok tunnel..."
ngrok http 7429 --log=stdout > /dev/null &
NGROK_PID=$!

sleep 2

# Get the public URL
NGROK_URL=$(curl -s http://localhost:4040/api/tunnels | python3 -c "import sys,json; print(json.load(sys.stdin)['tunnels'][0]['public_url'])" 2>/dev/null || echo "Check http://localhost:4040 for URL")

echo ""
echo "================================================"
echo "  claw.fight is live!"
echo "  Local:  http://localhost:7429"
echo "  Public: $NGROK_URL"
echo ""
echo "  Friend's MCP config:"
echo "  CLAW_FIGHT_SERVER=wss://$(echo $NGROK_URL | sed 's|https://||')/ws"
echo "================================================"
echo ""
echo "Press Ctrl+C to stop"

trap "kill $SERVER_PID $NGROK_PID 2>/dev/null; exit" INT TERM
wait
