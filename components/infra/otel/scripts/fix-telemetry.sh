#!/bin/bash

# Script to fix the telemetry stack issues
# This should be run after the LGTM stack is started

echo "Starting telemetry stack fixes..."

# Get the directory of this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Update the Grafana datasource
echo "Updating Grafana datasource..."
"$SCRIPT_DIR/update-grafana-datasource.sh"

# Update the Grafana dashboards
echo "Updating Grafana dashboards..."
"$SCRIPT_DIR/update-grafana-dashboards.sh"

# Run the health check to verify the fixes
echo "Running health check..."
"$SCRIPT_DIR/check-telemetry-health.sh"

echo "All fixes have been applied!" 