#!/bin/bash

# Script to verify the health of all telemetry components
# This can be called externally or mounted in the container

check_service() {
  local service_name=$1
  local port=$2
  local endpoint=$3
  local expected_status=$4

  echo "Checking $service_name on port $port..."
  
  # Try to connect with timeout to avoid hanging
  local status_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://$service_name:$port$endpoint" || echo "connection_failed")
  
  if [ "$status_code" = "connection_failed" ]; then
    if [ "$NETWORK_MODE" = "host" ]; then
      echo "⚠️ Could not connect to $service_name directly. This is expected if services are running in Docker."
      return 0
    else
      echo "❌ Failed to connect to $service_name"
      return 1
    fi
  elif [ "$status_code" -eq "$expected_status" ]; then
    echo "✅ $service_name is healthy (status: $status_code)"
    return 0
  else
    echo "❌ $service_name returned unexpected status: $status_code (expected: $expected_status)"
    return 1
  fi
}

check_docker_service() {
  local container_name=$1
  local port=$2
  local endpoint=$3
  local expected_status=$4

  echo "Checking service in container $container_name on port $port..."
  
  # Check if Docker is available
  if ! command -v docker &> /dev/null; then
    echo "⚠️ Docker command not found. Skipping container check."
    return 0
  fi

  # Check if the container is running
  if ! docker ps | grep -q "$container_name"; then
    echo "❌ Container $container_name is not running"
    return 1
  fi

  # Try to connect from inside the container
  local status_code=$(docker exec "$container_name" curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://localhost:$port$endpoint" || echo "connection_failed")
  
  if [ "$status_code" = "connection_failed" ]; then
    echo "❌ Failed to connect to service inside container $container_name"
    return 1
  elif [ "$status_code" -eq "$expected_status" ]; then
    echo "✅ Service in container $container_name is healthy (status: $status_code)"
    return 0
  else
    echo "❌ Service in container $container_name returned unexpected status: $status_code (expected: $expected_status)"
    return 1
  fi
}

# Try to load environment variables from .env if it exists and we're not in a container
if [ ! -f "/.dockerenv" ] && [ -f ".env" ]; then
  echo "Loading environment variables from .env file..."
  # shellcheck disable=SC1091
  source .env
fi

# Initialize variables with default values or from environment
OTEL_CONTAINER_NAME=${OTEL_CONTAINER_NAME:-midaz-otel-lgtm}
GRAFANA_PORT=${OTEL_LGTM_INTERNAL_PORT:-3000}
TEMPO_PORT=${TEMPO_PORT:-3200}
PROMETHEUS_PORT=${PROMETHEUS_PORT:-9090}
LOKI_PORT=${LOKI_PORT:-3100}
COLLECTOR_PORT=${OTEL_LGTM_METRICS_PORT:-8888}

# Determine if running inside container or outside
NETWORK_MODE="host"
if [ -f "/.dockerenv" ]; then
  NETWORK_MODE="container"
fi

echo "Running health checks in $NETWORK_MODE mode..."

failures=0

if [ "$NETWORK_MODE" = "container" ]; then
  # When inside container, check services directly
  # Check Grafana
  check_service "127.0.0.1" "$GRAFANA_PORT" "/api/health" 200 || ((failures++))

  # Check Tempo
  check_service "127.0.0.1" "$TEMPO_PORT" "/ready" 200 || ((failures++))

  # Check Prometheus (using the /-/healthy endpoint instead of /ready)
  check_service "127.0.0.1" "$PROMETHEUS_PORT" "/-/healthy" 200 || ((failures++))

  # Check Loki
  check_service "127.0.0.1" "$LOKI_PORT" "/ready" 200 || ((failures++))

  # Check OTEL Collector - Using metrics endpoint on port 8888
  check_service "127.0.0.1" "$COLLECTOR_PORT" "/metrics" 200 || ((failures++))
else
  # When running from host, check services through Docker
  # Check Grafana
  check_docker_service "$OTEL_CONTAINER_NAME" "$GRAFANA_PORT" "/api/health" 200 || ((failures++))

  # Check Tempo
  check_docker_service "$OTEL_CONTAINER_NAME" "$TEMPO_PORT" "/ready" 200 || ((failures++))

  # Check Prometheus (using the /-/healthy endpoint instead of /ready)
  check_docker_service "$OTEL_CONTAINER_NAME" "$PROMETHEUS_PORT" "/-/healthy" 200 || ((failures++))

  # Check Loki
  check_docker_service "$OTEL_CONTAINER_NAME" "$LOKI_PORT" "/ready" 200 || ((failures++))

  # Check OTEL Collector - Using metrics endpoint on port 8888
  check_docker_service "$OTEL_CONTAINER_NAME" "$COLLECTOR_PORT" "/metrics" 200 || ((failures++))
fi

# Display summary
echo ""
if [ $failures -eq 0 ]; then
  echo "✅ All telemetry components are healthy!"
  exit 0
else
  echo "❌ $failures telemetry component(s) are unhealthy."
  exit 1
fi