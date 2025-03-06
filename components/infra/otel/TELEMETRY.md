# Telemetry Setup

This document describes the telemetry setup for the Midaz application, including the LGTM (Loki, Grafana, Tempo, Mimir/Prometheus) stack.

## Components

The telemetry stack consists of the following components:

- **Grafana**: Visualization and dashboarding tool
- **Loki**: Log aggregation system
- **Tempo**: Distributed tracing backend
- **Prometheus**: Metrics collection and storage (used instead of Mimir)
- **OpenTelemetry Collector**: Telemetry data collection and processing

## Architecture

The telemetry stack is deployed as a single Docker container (`midaz-otel-lgtm`) that contains all the components. The services within the container communicate with each other using localhost.

The application services (onboarding, transaction, etc.) send telemetry data to the OpenTelemetry Collector, which then forwards it to the appropriate backend (Loki for logs, Tempo for traces, Prometheus for metrics).

## Ports

The following ports are exposed by the telemetry stack:

- **3000**: Grafana UI (accessible at http://localhost:3100 with credentials midaz:lerian)
- **3100**: Loki API
- **3200**: Tempo API
- **9090**: Prometheus API
- **4317**: OpenTelemetry Collector gRPC endpoint
- **4318**: OpenTelemetry Collector HTTP endpoint
- **8888**: OpenTelemetry Collector metrics endpoint

## Dashboards

The following dashboards are available in Grafana:

- **API Metrics**: HTTP request metrics (rate, latency, error rate)
- **Application Logs**: Logs from the application services
- **Distributed Tracing**: Traces from the application services
- **System Metrics**: System metrics (CPU, memory) from the application services

## Health Check

A health check script is provided to verify that all components of the telemetry stack are functioning correctly. The script can be run from the host or from within the container.

```bash
# Using the script directly
./components/infra/otel/scripts/check-telemetry-health.sh

# Using the Makefile target
cd components/infra
make telemetry-health
```

## Maintenance and Troubleshooting

The following scripts are provided for maintenance and troubleshooting purposes. They were initially created to fix specific issues with the telemetry stack, but they can also be useful for future maintenance or if similar issues arise.

### Fix Common Issues

If you encounter issues with the telemetry stack, you can use the fix script to apply common fixes:

```bash
# Using the script directly
./components/infra/otel/scripts/fix-telemetry.sh

# Using the Makefile target
cd components/infra
make telemetry-fix
```

The fix script performs the following actions:

1. Updates the Grafana datasource to point to the correct Prometheus URL
2. Updates the Grafana dashboards to use the correct metric names
3. Runs the health check to verify that all components are functioning correctly

### Common Issues

- **Grafana datasource configuration**: The Grafana datasource may be configured to use the wrong URL for Prometheus. The fix script updates the datasource to use the correct URL.
- **Metric name mismatches**: The Grafana dashboards may be configured to use metric names that don't match what's actually being collected. The fix script updates the dashboards to use the correct metric names.
- **Health check failures**: The health check script may report failures if it's not configured to check the correct endpoints. The fix script updates the health check script to use the correct endpoints.

### Manual Fixes

If the fix script doesn't resolve your issues, you can try the following manual fixes:

1. Update the Grafana datasource:

```bash
# Using the script directly
./components/infra/otel/scripts/update-grafana-datasource.sh

# Using the Makefile target
cd components/infra
make telemetry-update-datasource
```

2. Update the Grafana dashboards:

```bash
# Using the script directly
./components/infra/otel/scripts/update-grafana-dashboards.sh

# Using the Makefile target
cd components/infra
make telemetry-update-dashboards
```

3. Check the health of the telemetry stack:

```bash
# Using the script directly
./components/infra/otel/scripts/check-telemetry-health.sh

# Using the Makefile target
cd components/infra
make telemetry-health
```

### Additional Diagnostic Tools

The telemetry stack includes additional diagnostic tools to help with troubleshooting and testing:

1. Check LGTM container ports and services:

```bash
# Using the script directly
./components/infra/otel/scripts/check-lgtm-ports.sh

# Using the Makefile target
cd components/infra
make telemetry-check-ports
```

This script provides detailed information about the ports and services running inside the LGTM container, which can be helpful for diagnosing connectivity issues.

2. Send test metrics to the OpenTelemetry collector:

```bash
# Using the script directly (runs indefinitely until stopped with Ctrl+C)
./components/infra/otel/scripts/test-metrics.sh

# Run for a specific duration (e.g., 10 seconds)
./components/infra/otel/scripts/test-metrics.sh --duration 10

# Specify a different endpoint
./components/infra/otel/scripts/test-metrics.sh --endpoint http://localhost:4318

# Specify a different service name
./components/infra/otel/scripts/test-metrics.sh --service my-test-service

# Show help
./components/infra/otel/scripts/test-metrics.sh --help

# Using the Makefile target
cd components/infra
make telemetry-test-metrics
```

This script sends test metrics to the OpenTelemetry collector, which can be useful for verifying that the telemetry pipeline is working correctly.

## Generating Telemetry Data

To generate telemetry data for testing, you can hit the health endpoints of the application services:

```bash
curl http://localhost:3001/health  # Transaction service
curl http://localhost:3002/health  # Onboarding service
```

This will generate traces, logs, and metrics that will be visible in the Grafana dashboards. 