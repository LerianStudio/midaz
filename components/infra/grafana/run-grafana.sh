#!/bin/bash

source ./logging.sh

# Use environment variables with defaults
export GF_AUTH_ANONYMOUS_ENABLED=${GF_AUTH_ANONYMOUS_ENABLED:-false}
export GF_SECURITY_ADMIN_USER=${GF_SECURITY_ADMIN_USER:-midaz}
export GF_SECURITY_ADMIN_PASSWORD=${GF_SECURITY_ADMIN_PASSWORD:-lerian}

# Set up paths
export GF_PATHS_HOME=${GF_PATHS_HOME:-/data/grafana}
export GF_PATHS_DATA=${GF_PATHS_DATA:-/data/grafana/data}
export GF_PATHS_PLUGINS=${GF_PATHS_PLUGINS:-/data/grafana/plugins}
export GF_PATHS_PROVISIONING=${GF_PATHS_PROVISIONING:-/otel-lgtm/grafana/provisioning}

# Use telemetry config from environment
export GF_FEATURE_TOGGLES_ENABLE=tempoSearch,tempoBackendSearch,tracesToLogs,logsToTraces

# Configure logging
export GF_LOG_MODE=${GF_LOG_MODE:-console}
export GF_LOG_LEVEL=${GF_LOG_LEVEL:-info}

# Enable debug mode for troubleshooting
# export GF_LOG_LEVEL=debug

# Start Grafana in the background
cd ./grafana || exit
run_with_logging "Grafana ${GRAFANA_VERSION}" "${ENABLE_LOGS_GRAFANA:-false}" ./bin/grafana server &

# Store the Grafana PID
GRAFANA_PID=$!

# Run the initialization script in the background
(
  # Wait for Grafana to be ready
  sleep 30
  
  # Run the initialization script
  /otel-lgtm/grafana/init-grafana.sh
) &

# Wait for Grafana to exit
wait $GRAFANA_PID