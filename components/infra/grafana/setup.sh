#!/bin/bash

# Consolidated script for Grafana setup, dashboard provisioning, and initialization
# This replaces run-grafana.sh, init-grafana.sh, run-all-once.sh, and logging.sh

# ======== LOGGING FUNCTIONS ========
run_with_logging() {
  SERVICE_NAME=$1
  ENABLE_LOGS=$2
  shift 2
  
  if [ "$ENABLE_LOGS" = "true" ]; then
    echo "Starting $SERVICE_NAME with logging enabled"
    "$@" 2>&1 | while read -r line; do
      echo "[$SERVICE_NAME] $line"
    done
  else
    echo "Starting $SERVICE_NAME with logging disabled"
    "$@" > /dev/null 2>&1
  fi
}

# ======== SETUP FUNCTIONS ========
setup_grafana_config() {
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
}

setup_directories() {
  # Create required directories
  mkdir -p ${GF_PATHS_DATA}/dashboards
  mkdir -p ${GF_PATHS_PROVISIONING}/dashboards
  mkdir -p ${GF_PATHS_PROVISIONING}/datasources
}

copy_dashboards() {
  echo "Copying dashboard files..."
  # The dashboards are already mounted at /otel-lgtm/grafana/dashboards 
  # via the Docker Compose volume mapping
  
  # Make sure the dashboards directory in data exists
  mkdir -p ${GF_PATHS_DATA}/dashboards
  
  # Copy dashboards to data directory if needed
  cp /otel-lgtm/grafana/dashboards/*.json ${GF_PATHS_DATA}/dashboards/ 2>/dev/null || true
}

# ======== INITIALIZATION FUNCTIONS ========
wait_for_grafana() {
  echo "Waiting for Grafana to be ready..."
  MAX_RETRIES=30
  RETRY_INTERVAL=2
  RETRY_COUNT=0

  until curl -s "http://localhost:3000/api/health" > /dev/null || [ ${RETRY_COUNT} -ge ${MAX_RETRIES} ]; do
    echo "Waiting for Grafana... (Attempt ${RETRY_COUNT}/${MAX_RETRIES})"
    sleep ${RETRY_INTERVAL}
    RETRY_COUNT=$((RETRY_COUNT+1))
  done

  if [ ${RETRY_COUNT} -ge ${MAX_RETRIES} ]; then
    echo "Failed to connect to Grafana after ${MAX_RETRIES} retries."
    return 1
  fi

  echo "Grafana is ready!"
  return 0
}

reload_provisioning() {
  echo "Reloading provisioning..."
  curl -s -X POST -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" \
    "http://localhost:3000/api/admin/provisioning/dashboards/reload"
  curl -s -X POST -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" \
    "http://localhost:3000/api/admin/provisioning/datasources/reload"
}

create_dashboard_folder() {
  echo "Creating Midaz folder..."
  FOLDER_RESULT=$(curl -s -X POST -H "Content-Type: application/json" \
    -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" \
    -d '{"title":"Midaz", "uid":"midaz"}' \
    "http://localhost:3000/api/folders")
  
  echo "Folder creation result: $FOLDER_RESULT"
}

import_dashboards() {
  echo "Importing dashboards directly..."
  for dashboard in /otel-lgtm/grafana/dashboards/*.json; do
    DASHBOARD_NAME=$(basename "$dashboard" .json)
    echo "Importing $DASHBOARD_NAME..."
    
    # Update dashboard to remove any existing ID and UID
    TMP_DASHBOARD="/tmp/$(basename "$dashboard")"
    jq 'del(.id) | del(.uid)' "$dashboard" > "$TMP_DASHBOARD"
    
    # Create dashboard JSON payload with folder
    IMPORT_PAYLOAD=$(jq -n \
      --arg dashboard "$(cat "$TMP_DASHBOARD")" \
      '{"dashboard": $dashboard | fromjson, "folderUid": "midaz", "overwrite": true}')
    
    # Import the dashboard
    RESULT=$(curl -s -X POST -H "Content-Type: application/json" \
      -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" \
      -d "$IMPORT_PAYLOAD" \
      "http://localhost:3000/api/dashboards/db")
    
    echo "Result for $DASHBOARD_NAME: $RESULT"
  done
}

# ======== MAIN EXECUTION ========
main() {
  echo "Starting Grafana setup..."

  # Setup configuration
  setup_grafana_config
  setup_directories
  copy_dashboards
  
  # Start Grafana in background
  cd /otel-lgtm/grafana || exit 1
  run_with_logging "Grafana" "${ENABLE_LOGS_GRAFANA:-false}" ./bin/grafana server &
  GRAFANA_PID=$!

  # Run initialization in background once Grafana is up
  (
    # Wait for Grafana to be fully up
    if wait_for_grafana; then
      reload_provisioning
      create_dashboard_folder
      import_dashboards
      echo "Grafana configuration completed successfully!"
    else
      echo "Failed to initialize Grafana."
    fi
  ) &

  # Wait for Grafana to exit
  wait $GRAFANA_PID
}

main "$@" 