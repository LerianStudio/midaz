#!/bin/bash

# Script to send test metrics to OpenTelemetry collector for testing
# Uses curl to send metrics via HTTP

set -e

# OTEL collector endpoint - allow configuring via env var or use default
ENDPOINT=${OTEL_EXPORTER_OTLP_ENDPOINT:-"http://localhost:4318"}
SERVICE_NAME=${OTEL_SERVICE_NAME:-"midaz-test-service"}

# Service version and environment - for better metrics labeling
SERVICE_VERSION=${OTEL_SERVICE_VERSION:-"1.0.0"}
ENV=${OTEL_ENV:-"development"}

# Default duration in seconds (0 means run indefinitely)
DURATION=${DURATION:-0}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -d|--duration)
      DURATION="$2"
      shift 2
      ;;
    -e|--endpoint)
      ENDPOINT="$2"
      shift 2
      ;;
    -s|--service)
      SERVICE_NAME="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [options]"
      echo "Options:"
      echo "  -d, --duration SECONDS   Run for specified duration (default: run indefinitely)"
      echo "  -e, --endpoint URL       OTLP endpoint URL (default: http://localhost:4318)"
      echo "  -s, --service NAME       Service name (default: midaz-test-service)"
      echo "  -h, --help               Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Function to send a metric
send_metric() {
  local metric_name=$1
  local metric_value=$2
  local timestamp=$(date +%s)000000000  # Current time in nanoseconds

  # Create OTLP metrics payload
  cat > /tmp/metric.json << EOF
{
  "resourceMetrics": [
    {
      "resource": {
        "attributes": [
          {
            "key": "service.name",
            "value": {
              "stringValue": "${SERVICE_NAME}"
            }
          },
          {
            "key": "service.version",
            "value": {
              "stringValue": "${SERVICE_VERSION}"
            }
          },
          {
            "key": "deployment.environment",
            "value": {
              "stringValue": "${ENV}"
            }
          }
        ]
      },
      "scopeMetrics": [
        {
          "metrics": [
            {
              "name": "${metric_name}",
              "gauge": {
                "dataPoints": [
                  {
                    "asInt": ${metric_value},
                    "timeUnixNano": "${timestamp}",
                    "attributes": [
                      {
                        "key": "test",
                        "value": {
                          "stringValue": "true"
                        }
                      }
                    ]
                  }
                ]
              }
            }
          ]
        }
      ]
    }
  ]
}
EOF

  # Send metrics via OTLP HTTP
  curl -s -X POST "${ENDPOINT}/v1/metrics" \
    -H "Content-Type: application/json" \
    -d @/tmp/metric.json > /dev/null

  echo "Sent metric ${metric_name}=${metric_value}"
}

# Check if we can connect to the endpoint
if ! curl -s --head "${ENDPOINT}" > /dev/null; then
  echo "❌ Cannot connect to ${ENDPOINT}"
  echo "Make sure the OTEL collector is running and reachable"
  exit 1
fi

echo "✅ Successfully connected to ${ENDPOINT}"
echo "Sending test metrics from ${SERVICE_NAME} (${SERVICE_VERSION}) in ${ENV} environment"

if [ "$DURATION" -eq 0 ]; then
  echo "Running indefinitely. Press Ctrl+C to stop"
else
  echo "Running for ${DURATION} seconds..."
  END_TIME=$(($(date +%s) + DURATION))
fi

# Generate and send metrics matching our system metrics dashboards
i=0
while true; do
  # Check if we've reached the duration limit
  if [ "$DURATION" -ne 0 ] && [ $(date +%s) -ge $END_TIME ]; then
    echo "Reached specified duration of ${DURATION} seconds. Exiting."
    break
  fi

  # CPU usage metrics
  cpu_value=$((RANDOM % 101))
  send_metric "system.cpu.usage" "${cpu_value}"
  send_metric "node_cpu_usage_percentage" "${cpu_value}"  # Standard name
  
  # Memory usage metrics
  mem_value=$((RANDOM % 101))
  send_metric "system.mem.usage" "${mem_value}"
  send_metric "node_memory_used_percentage" "${mem_value}"  # Standard name
  
  # HTTP request count
  requests=$((RANDOM % 10 + 1))
  send_metric "http.server.request_count" "${requests}"
  
  # Add status code dimension
  for status in 200 404 500; do
    reqs=$((RANDOM % 5))
    send_metric "http.server.request_count{status_code=\"${status}\"}" "${reqs}"
  done
  
  # HTTP latency
  for percentile in 50 95 99; do
    latency=$((percentile * 10 + RANDOM % 200))
    send_metric "http.server.request_duration_milliseconds{quantile=\"0.${percentile}\"}" "${latency}"
  done
  
  # Counter that increases (for metrics with trends)
  i=$((i+1))
  send_metric "test.counter" "${i}"
  
  echo "-------------------------------------------------------------"
  echo "Sent batch of metrics at $(date)"
  echo "-------------------------------------------------------------"
  
  # Wait before sending next batch
  sleep 0.5
done