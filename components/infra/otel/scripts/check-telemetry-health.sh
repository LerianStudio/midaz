#!/bin/bash

# This script checks the health of the telemetry stack components
# It can be run inside the container or on the host

# Load environment variables from .env file if it exists
if [ -f .env ]; then
  echo "Loading environment variables from .env file..."
  source .env
fi

# Determine if we're running inside the container or on the host
if [ -f /.dockerenv ]; then
  echo "Running health checks in container mode..."
  HOST="localhost"
  GRAFANA_PORT="3000"  # Grafana runs on port 3000 inside the container
else
  echo "Running health checks in host mode..."
  HOST="localhost"
  GRAFANA_PORT="3100"  # Grafana is mapped to port 3100 on the host
fi

# Check if a service is healthy
check_service() {
  local container=$1
  local port=$2
  local url=$3
  local expected_status=${4:-200}
  local service_name=${5:-"Service"}
  local accept_multiple_statuses=${6:-false}
  local additional_status=${7:-0}

  echo "Checking $service_name in container $container on port $port..."
  
  # Use curl to check if the service is responding
  status=$(curl -s -o /dev/null -w "%{http_code}" http://$HOST:$port$url)
  
  if [ "$status" -eq "$expected_status" ] || ( [ "$accept_multiple_statuses" = true ] && [ "$status" -eq "$additional_status" ] ); then
    echo "✅ $service_name in container $container is healthy (status: $status)"
    return 0
  else
    if [ "$accept_multiple_statuses" = true ]; then
      echo "❌ $service_name in container $container is unhealthy (status: $status, expected: $expected_status or $additional_status)"
    else
      echo "❌ $service_name in container $container is unhealthy (status: $status, expected: $expected_status)"
    fi
    return 1
  fi
}

# Check all services
CONTAINER="midaz-otel-lgtm"
UNHEALTHY_COUNT=0

# Check Grafana
check_service $CONTAINER $GRAFANA_PORT "/" 302 "Grafana" false 0 || ((UNHEALTHY_COUNT++))

# Check Tempo
check_service $CONTAINER 3200 "/ready" 200 "Tempo" false 0 || ((UNHEALTHY_COUNT++))

# Check Prometheus
check_service $CONTAINER 9090 "/-/healthy" 200 "Prometheus" false 0 || ((UNHEALTHY_COUNT++))

# Report overall health
if [ $UNHEALTHY_COUNT -eq 0 ]; then
  echo -e "\n✅ All telemetry components are healthy!"
  exit 0
else
  echo -e "\n❌ $UNHEALTHY_COUNT telemetry components are unhealthy!"
  exit 1
fi