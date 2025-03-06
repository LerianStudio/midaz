#!/bin/bash

# Script to update the Grafana dashboards to use the correct metric names
# This should be run after the LGTM stack is started

# Default values
GRAFANA_URL=${GRAFANA_URL:-http://localhost:3100}
GRAFANA_USER=${GRAFANA_USER:-midaz}
GRAFANA_PASSWORD=${GRAFANA_PASSWORD:-lerian}

echo "Updating Grafana dashboards to use the correct metric names..."

# Function to update a dashboard
update_dashboard() {
  local dashboard_uid=$1
  local old_metric=$2
  local new_metric=$3
  
  echo "Updating dashboard $dashboard_uid: $old_metric -> $new_metric"
  
  # Get the current dashboard
  local dashboard_json=$(curl -s -u "$GRAFANA_USER:$GRAFANA_PASSWORD" "$GRAFANA_URL/api/dashboards/uid/$dashboard_uid")
  
  # Replace the metric name in the dashboard JSON
  local updated_json=$(echo "$dashboard_json" | sed "s/$old_metric/$new_metric/g")
  
  # Extract the dashboard part
  local dashboard_part=$(echo "$updated_json" | jq '.dashboard')
  
  # Update the dashboard
  curl -X POST \
    -H "Content-Type: application/json" \
    -d "{
      \"dashboard\": $dashboard_part,
      \"overwrite\": true
    }" \
    -u "$GRAFANA_USER:$GRAFANA_PASSWORD" \
    "$GRAFANA_URL/api/dashboards/db"
}

# Update the API Metrics dashboard
update_dashboard "api-metrics" "http_server_request_count" "http_server_request_count_total"

# Update the System Metrics dashboard
update_dashboard "system-metrics" "system_cpu_usage" "system_cpu_usage_percentage"
update_dashboard "system-metrics" "system_mem_usage" "system_mem_usage_percentage"

echo "Done." 