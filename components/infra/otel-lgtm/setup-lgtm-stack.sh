#!/bin/bash

# Check if LGTM stack is running
if ! docker ps | grep -q midaz-otel-lgtm; then
  echo "Error: LGTM stack container is not running"
  exit 1
fi

echo "Copying custom OpenTelemetry collector configuration to container..."
docker cp config/custom-otelcol-config.yaml midaz-otel-lgtm:/otel-lgtm/custom-otelcol-config.yaml

echo "Copying Grafana dashboard to container..."
docker cp grafana/dashboards/midaz-business-metrics-dashboard.json midaz-otel-lgtm:/otel-lgtm/grafana/provisioning/dashboards/

echo "Creating dashboard provisioning configuration..."
docker cp grafana/dashboards/dashboard.yaml midaz-otel-lgtm:/otel-lgtm/grafana/provisioning/dashboards/

echo "Stopping OpenTelemetry collector services for reconfiguration..."
docker exec midaz-otel-lgtm pkill otelcol-contrib || true
sleep 2

echo "Starting OpenTelemetry collector with the new configuration..."
docker exec -d midaz-otel-lgtm bash -c "cd /otel-lgtm && ENABLE_LOGS_OTELCOL=true ./otelcol-contrib/otelcol-contrib --config=file:./custom-otelcol-config.yaml > /tmp/otelcol.log 2>&1"

echo "Restarting Grafana to apply dashboard changes..."
docker exec midaz-otel-lgtm pkill grafana-server || true
sleep 2
docker exec -d midaz-otel-lgtm bash -c "cd /otel-lgtm && ./run-grafana.sh"

echo "Installation completed!"
echo
echo "To check collector logs, run: docker exec midaz-otel-lgtm cat /tmp/otelcol.log"
echo "Access Grafana at: http://localhost:3100"
echo
echo "You may need to wait a few minutes for metrics to start appearing in Grafana." 