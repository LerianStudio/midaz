#!/bin/bash

# Helper script to check which ports are open inside the LGTM container
# Run this inside the container to see what services are available

# Check container is running
CONTAINER_NAME=${OTEL_CONTAINER_NAME:-midaz-otel-lgtm}

if ! docker ps | grep -q $CONTAINER_NAME; then
  echo "❌ LGTM container ($CONTAINER_NAME) is not running"
  echo "Start the container with docker-compose up -d $CONTAINER_NAME"
  exit 1
fi

echo "✅ LGTM container ($CONTAINER_NAME) is running"
echo
echo "Checking listening ports inside the container..."
echo

# Execute netstat inside container (or alternatives)
docker exec $CONTAINER_NAME sh -c "netstat -tulpn 2>/dev/null || ss -tulpn 2>/dev/null || echo 'No network tools available'" | grep -v ^Proto | sort -k4

echo
echo "Checking services inside container..."
echo
docker exec $CONTAINER_NAME sh -c "ps aux | grep -v grep | grep -E 'tempo|loki|mimir|grafana|otel|prometheus'"

echo
echo "Checking if Prometheus is available..."
docker exec $CONTAINER_NAME sh -c "ls -la /opt/ || ls -la /prometheus/ 2>/dev/null"

echo
echo "Testing Grafana API endpoints..."
echo "Available datasources:"
# Use the correct credentials for Grafana
GRAFANA_USER=${GRAFANA_USER:-midaz}
GRAFANA_PASSWORD=${GRAFANA_PASSWORD:-lerian}
curl -s -u "$GRAFANA_USER:$GRAFANA_PASSWORD" http://localhost:${OTEL_LGTM_EXTERNAL_PORT:-3100}/api/datasources 2>/dev/null | grep -o '"name":"[^"]*"' || echo "Cannot fetch datasources (try with correct credentials)"

echo
echo "Testing Prometheus API directly..."
echo -n "Testing Prometheus API: "
# Test Prometheus API inside the container
docker exec $CONTAINER_NAME curl -s --connect-timeout 1 http://localhost:9090/api/v1/status/buildinfo 2>/dev/null | grep -q version && echo "✅ Prometheus API found" || echo "❌ No Prometheus API found"

echo
echo "Testing accessibility of Grafana..."
curl -s -o /dev/null -w "Grafana HTTP status: %{http_code}\n" http://localhost:${OTEL_LGTM_EXTERNAL_PORT:-3100} || echo "Cannot connect to Grafana"

echo
echo "Testing OTLP collector metrics endpoint..."
# Test metrics endpoint inside the container
docker exec $CONTAINER_NAME curl -s -o /dev/null -w "OTLP metrics endpoint HTTP status: %{http_code}\n" http://localhost:8888/metrics 2>/dev/null || echo "Cannot connect to OTLP metrics endpoint"

echo
echo "Testing OTLP collector receiver endpoint..."
# Test receiver endpoint inside the container
docker exec $CONTAINER_NAME curl -s -o /dev/null -w "OTLP receiver endpoint HTTP status: %{http_code}\n" http://localhost:4318 2>/dev/null || echo "Cannot connect to OTLP receiver endpoint"