#!/bin/bash

# This script initializes Grafana after it's started
# It ensures that datasources and dashboards are properly loaded

echo "Initializing Grafana..."

# Wait for Grafana to be fully up
# Using curl to check if the API is responding
MAX_RETRIES=30
RETRY_INTERVAL=2
GRAFANA_URL="http://localhost:3000"
RETRY_COUNT=0

# Function to check if Grafana is up
check_grafana() {
    curl -s -o /dev/null -w "%{http_code}" -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" "${GRAFANA_URL}/api/health"
}

# Wait for Grafana to be up
until [ "$(check_grafana)" -eq 200 ] || [ ${RETRY_COUNT} -ge ${MAX_RETRIES} ]; do
    echo "Waiting for Grafana to be up... (Attempt ${RETRY_COUNT}/${MAX_RETRIES})"
    sleep ${RETRY_INTERVAL}
    RETRY_COUNT=$((RETRY_COUNT+1))
done

if [ ${RETRY_COUNT} -ge ${MAX_RETRIES} ]; then
    echo "Failed to connect to Grafana after ${MAX_RETRIES} retries."
    exit 1
fi

echo "Grafana is up and running!"

# Force provisioning reload if needed
curl -s -X POST -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" "${GRAFANA_URL}/api/admin/provisioning/dashboards/reload"
curl -s -X POST -u "${GF_SECURITY_ADMIN_USER}:${GF_SECURITY_ADMIN_PASSWORD}" "${GRAFANA_URL}/api/admin/provisioning/datasources/reload"

echo "Grafana initialization completed successfully!" 