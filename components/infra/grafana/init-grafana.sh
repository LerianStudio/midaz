#!/bin/bash

# This script initializes Grafana with the necessary datasources and dashboards
# It should be run after Grafana is started

# Set default values for environment variables
GRAFANA_URL=${GRAFANA_URL:-http://localhost:3100}
GRAFANA_USER=${GRAFANA_USER:-midaz}
GRAFANA_PASSWORD=${GRAFANA_PASSWORD:-lerian}
MAX_RETRIES=30
RETRY_INTERVAL=5

echo "Waiting for Grafana to be ready..."
# Wait for Grafana to be ready
for i in $(seq 1 $MAX_RETRIES); do
  if curl -s -o /dev/null -w "%{http_code}" $GRAFANA_URL/api/health | grep -q "200"; then
    echo "Grafana is ready!"
    break
  fi
  
  if [ $i -eq $MAX_RETRIES ]; then
    echo "Timed out waiting for Grafana to be ready"
    exit 1
  fi
  
  echo "Waiting for Grafana to be ready... (Attempt $i/$MAX_RETRIES)"
  sleep $RETRY_INTERVAL
done

# Function to create a datasource
create_datasource() {
  local datasource_file=$1
  local datasource_name=$(grep -o '"name": "[^"]*"' $datasource_file | head -1 | cut -d'"' -f4)
  
  echo "Creating datasource: $datasource_name"
  
  curl -s -X POST -H "Content-Type: application/json" -d @$datasource_file \
    -u "$GRAFANA_USER:$GRAFANA_PASSWORD" \
    $GRAFANA_URL/api/datasources
  
  echo ""
}

# Function to create a dashboard
create_dashboard() {
  local dashboard_file=$1
  local dashboard_name=$(grep -o '"title": "[^"]*"' $dashboard_file | head -1 | cut -d'"' -f4)
  
  echo "Creating dashboard: $dashboard_name"
  
  # Wrap the dashboard JSON in the required format for the API
  local temp_file=$(mktemp)
  echo '{"dashboard":' > $temp_file
  cat $dashboard_file >> $temp_file
  echo ', "overwrite": true, "message": "Automated dashboard creation"}' >> $temp_file
  
  curl -s -X POST -H "Content-Type: application/json" -d @$temp_file \
    -u "$GRAFANA_USER:$GRAFANA_PASSWORD" \
    $GRAFANA_URL/api/dashboards/db
  
  rm $temp_file
  echo ""
}

# Create datasources
echo "Creating datasources..."
for datasource in /otel-lgtm/grafana/provisioning/datasources/*.yaml; do
  create_datasource $datasource
done

# Create dashboards
echo "Creating dashboards..."
for dashboard in /otel-lgtm/grafana/dashboards/*.json; do
  create_dashboard $dashboard
done

echo "Grafana initialization complete!" 