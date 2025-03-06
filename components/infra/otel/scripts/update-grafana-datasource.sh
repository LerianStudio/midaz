#!/bin/bash

# Script to update the Grafana datasource to point to the correct Prometheus URL
# This should be run after the LGTM stack is started

# Default values
GRAFANA_URL=${GRAFANA_URL:-http://localhost:3100}
GRAFANA_USER=${GRAFANA_USER:-midaz}
GRAFANA_PASSWORD=${GRAFANA_PASSWORD:-lerian}
PROMETHEUS_URL=${PROMETHEUS_URL:-http://localhost:9090}

echo "Updating Grafana datasource to point to Prometheus at $PROMETHEUS_URL..."

# Update the Mimir datasource to point to Prometheus
curl -X PUT \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Prometheus\",
    \"type\": \"prometheus\",
    \"url\": \"$PROMETHEUS_URL\",
    \"access\": \"proxy\",
    \"isDefault\": true,
    \"jsonData\": {
      \"prometheusType\": \"Prometheus\"
    }
  }" \
  -u "$GRAFANA_USER:$GRAFANA_PASSWORD" \
  "$GRAFANA_URL/api/datasources/uid/mimir"

# Check if the update was successful
if [ $? -eq 0 ]; then
  echo "✅ Grafana datasource updated successfully!"
else
  echo "❌ Failed to update Grafana datasource."
  exit 1
fi

echo "Done." 