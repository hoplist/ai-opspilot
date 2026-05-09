#!/bin/sh
set -eu

service_name="${RCA_SERVICE:-backend}"

if [ "$service_name" = "backend" ]; then
  exec python backend_server.py --host 0.0.0.0 --port 18080
fi

if [ "$service_name" = "mcp" ]; then
  exec python auto_inspection_mcp.py --host 0.0.0.0 --port 18081
fi

echo "Unknown RCA_SERVICE: $service_name" >&2
exit 1
