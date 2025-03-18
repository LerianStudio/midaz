# Midaz Telemetry with LGTM Stack

This directory contains the configuration for the LGTM (Loki, Grafana, Tempo, Mimir) stack used for telemetry in the Midaz platform.

## Components

- **OpenTelemetry Collector**: Collects and processes metrics, traces, and logs from our services
- **Grafana**: Visualization platform for monitoring and observability
- **Prometheus**: Time series database for storing metrics
- **Tempo**: Distributed tracing backend
- **Loki**: Log aggregation system

## Directory Structure

```
otel-lgtm/
├── config/
│   └── custom-otelcol-config.yaml        # Custom OpenTelemetry Collector configuration
├── grafana/
│   └── dashboards/                       # Grafana dashboards
│       ├── dashboard.yaml                # Dashboard provisioning configuration
│       └── midaz-business-metrics-dashboard.json   # Business metrics dashboard
└── setup-lgtm-stack.sh                   # Script to setup LGTM stack
```

## How to Use

### Starting the Stack

The LGTM stack is started automatically when you run the infrastructure using Docker Compose:

```bash
cd components/infra
docker-compose up -d
```

### Applying Custom Configuration

If you need to apply the custom configuration to an already running stack:

```bash
cd components/infra
make restart-lgtm
```

Or manually:

```bash
cd components/infra/otel-lgtm
./setup-lgtm-stack.sh
```

### Accessing the Dashboards

- **Grafana**: http://localhost:3100 (default credentials: midaz:lerian)

## Metrics Overview

The configured telemetry system collects the following metrics:

### Transaction Metrics
- `business_transaction_count_total`: Counts of transaction operations
- `business_transaction_duration_milliseconds`: Duration of transaction operations

### Onboarding Metrics
- `business_onboarding_count_total`: Counts of onboarding operations by entity type
- `business_onboarding_duration_milliseconds`: Duration of onboarding operations
- `business_onboarding_errors_count`: Count of errors during onboarding operations

## Adding New Dashboards

To add a new dashboard:

1. Create a JSON dashboard file and place it in `grafana/dashboards/`
2. Restart the LGTM stack using `make restart-lgtm`

## Troubleshooting

If the metrics don't appear in Grafana:

1. Check the OpenTelemetry Collector logs:
   ```bash
   docker exec midaz-otel-lgtm cat /tmp/otelcol.log
   ```

2. Ensure your services are configured to send metrics to `midaz-otel-lgtm:4317`

3. Check Prometheus metrics endpoint:
   ```bash
   curl http://localhost:8889/metrics | grep business
   ``` 