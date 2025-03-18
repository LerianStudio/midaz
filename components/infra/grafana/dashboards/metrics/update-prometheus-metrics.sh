#!/bin/bash

# Script to update the Prometheus metrics list in prometheus-metrics.md
# Usage: ./update-prometheus-metrics.sh

# Set variables
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
METRICS_FILE="${SCRIPT_DIR}/prometheus-metrics.md"
PROMETHEUS_URL="http://localhost:9090"

# Check if curl and jq are available
if ! command -v curl &> /dev/null; then
    echo "Error: curl is not installed"
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "Error: jq is not installed"
    exit 1
fi

# Function to check if Prometheus is accessible
check_prometheus() {
    if ! curl -s "${PROMETHEUS_URL}/-/healthy" &> /dev/null; then
        echo "Error: Cannot connect to Prometheus at ${PROMETHEUS_URL}"
        echo "Make sure Prometheus is running and accessible."
        exit 1
    fi
}

# Fetch all metric names from Prometheus
fetch_metrics() {
    echo "Fetching metrics from Prometheus..."
    METRICS=$(curl -s "${PROMETHEUS_URL}/api/v1/label/__name__/values" | jq -r '.data[]')
    
    if [ -z "$METRICS" ]; then
        echo "Error: Failed to retrieve metrics from Prometheus"
        exit 1
    fi
    
    echo "Successfully retrieved $(echo "$METRICS" | wc -l | tr -d ' ') metrics"
}

# Categorize metrics based on their prefixes
categorize_metrics() {
    # Create temporary arrays for each category
    BUSINESS_METRICS=$(echo "$METRICS" | grep "^business_" | sort)
    HTTP_METRICS=$(echo "$METRICS" | grep "^http_" | sort)
    OTEL_METRICS=$(echo "$METRICS" | grep "^otelcol_" | sort)
    PROM_METRICS=$(echo "$METRICS" | grep -E "^(promhttp_|scrape_|up$)" | sort)
    SYSTEM_METRICS=$(echo "$METRICS" | grep -E "^(system_|service_|target_)" | sort)
    TRACING_METRICS=$(echo "$METRICS" | grep "^traces_" | sort)
    
    # Find any uncategorized metrics
    OTHER_METRICS=$(echo "$METRICS" | grep -v -E "^(business_|http_|otelcol_|promhttp_|scrape_|system_|service_|target_|traces_|up$)" | sort)
}

# Generate the markdown file with the categorized metrics
generate_markdown() {
    echo "Generating markdown file..."
    
    # Create header
    cat > "$METRICS_FILE" << EOL
# Prometheus Metrics

This file lists all metrics currently available in Prometheus as part of the LGTM stack.
Last updated: $(date)

EOL

    # Add business metrics section
    if [ ! -z "$BUSINESS_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## Business Metrics
$(echo "$BUSINESS_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    # Add HTTP metrics section
    if [ ! -z "$HTTP_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## HTTP Server Metrics
$(echo "$HTTP_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    # Add OpenTelemetry metrics section
    if [ ! -z "$OTEL_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## OpenTelemetry Collector Metrics
$(echo "$OTEL_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    # Add Prometheus internal metrics section
    if [ ! -z "$PROM_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## Prometheus Internal Metrics
$(echo "$PROM_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    # Add System metrics section
    if [ ! -z "$SYSTEM_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## System and Service Metrics
$(echo "$SYSTEM_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    # Add Tracing metrics section
    if [ ! -z "$TRACING_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## Tracing Metrics
$(echo "$TRACING_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    # Add Other metrics section if any
    if [ ! -z "$OTHER_METRICS" ]; then
        cat >> "$METRICS_FILE" << EOL
## Other Metrics
$(echo "$OTHER_METRICS" | sed 's/^/- `/' | sed 's/$/` /')

EOL
    fi
    
    echo "Metrics file updated successfully at: $METRICS_FILE"
}

# Main execution
echo "Starting Prometheus metrics update..."
check_prometheus
fetch_metrics
categorize_metrics
generate_markdown
echo "Done!" 