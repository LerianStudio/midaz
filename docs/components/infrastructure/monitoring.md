# Monitoring

**Navigation:** [Home](../../) > [Infrastructure](./README.md) > Monitoring

This document describes the monitoring infrastructure used in the Midaz platform, including metrics collection, visualization, alerts, and best practices.

## Table of Contents

- [Overview](#overview)
- [Monitoring Stack](#monitoring-stack)
- [Metrics and Logging](#metrics-and-logging)
- [Dashboards](#dashboards)
- [Alerts](#alerts)
- [Setup and Configuration](#setup-and-configuration)
- [Using the Monitoring Tools](#using-the-monitoring-tools)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)
- [References](#references)

## Overview

Midaz uses a comprehensive monitoring infrastructure to track the health, performance, and behavior of all platform components. The monitoring system provides visibility into:

- Service availability and health
- Performance metrics and latency
- Resource utilization
- Error rates and exceptions
- Message queue status
- Database performance
- End-to-end transaction processing
- Business metrics

This observability allows for proactive identification of issues, performance optimization, and capacity planning.

## Monitoring Stack

The Midaz monitoring stack is built on several integrated tools:

### Grafana + OpenTelemetry (OTEL-LGTM)

The platform uses an observability stack called "otel-lgtm" (OpenTelemetry + Loki, Grafana, Tempo, and Mimir):

- **Grafana**: Visualization dashboards for metrics, logs, and traces
- **OpenTelemetry**: Collection and instrumentation framework for metrics, traces, and logs
- **Loki**: Log aggregation and querying system
- **Tempo**: Distributed tracing backend
- **Mimir**: Scalable metrics storage

This integrated stack provides a complete observability solution with correlation between metrics, logs, and traces.

### Health Checks

All critical infrastructure services include health checks to monitor availability:

- **PostgreSQL**: Primary and replica database health checks
- **MongoDB**: Database cluster health monitoring
- **RabbitMQ**: Message broker availability checks
- **Redis**: Cache service health monitoring

### Service Instrumentation

Applications in the Midaz platform are instrumented using the OpenTelemetry SDK, which provides:

- Automatic instrumentation of HTTP requests and responses
- Database query monitoring
- Runtime metrics (memory, CPU)
- Custom business metrics
- Distributed tracing
- Structured logging

## Metrics and Logging

### Key Metrics

The monitoring system collects the following key metrics:

#### System Metrics
- CPU, memory, disk, and network utilization
- Container resource usage
- JVM/runtime metrics (for applicable services)

#### Database Metrics
- Query execution time
- Connection pool utilization
- Transaction throughput
- Replication lag (PostgreSQL)
- Replica set status (MongoDB)

#### Message Queue Metrics
- Queue depth and throughput
- Message processing rates
- Consumer lag
- Failed deliveries

#### Application Metrics
- Request rate and latency
- Error rates and types
- Endpoint performance
- Business transaction metrics
- Success/failure rates

### Logging

Midaz uses structured logging throughout the platform:

- **Log Levels**: DEBUG, INFO, WARN, ERROR, FATAL
- **Log Format**: JSON format with standardized fields
- **Log Collection**: All logs are collected by Loki for centralized analysis
- **Correlation**: Logs include trace and span IDs for correlation with traces

## Dashboards

Grafana dashboards are organized by component and function:

### System Dashboards
- **Infrastructure Overview**: Overall system health
- **Resource Utilization**: CPU, memory, disk, and network monitoring
- **Container Metrics**: Container-level resource monitoring

### Service Dashboards
- **Service Health**: Per-service health and performance metrics
- **Endpoint Performance**: API endpoint response times and error rates
- **Service Dependencies**: Service interaction monitoring

### Database Dashboards
- **PostgreSQL Metrics**: Primary and replica database performance
- **MongoDB Metrics**: Document database performance
- **Redis Metrics**: Cache performance

### Queue Dashboards
- **RabbitMQ Overview**: Queue health and performance
- **Message Flow**: Message processing tracking

### Business Metrics Dashboards
- **Transaction Processing**: Financial transaction metrics
- **User Activity**: User interaction metrics

## Alerts

Alert rules are configured in Grafana for automated monitoring:

### Infrastructure Alerts
- Database availability
- Message queue connectivity
- Service health checks

### Performance Alerts
- High latency in critical services
- Slow database queries
- API endpoint response times

### Error Alerts
- High error rates
- Failed transactions
- Infrastructure failures

### Capacity Alerts
- Resource utilization thresholds
- Queue depth warnings
- Database connection pool saturation

## Setup and Configuration

### Setting Up the Monitoring Stack

The monitoring infrastructure is part of the standard Midaz deployment and is managed through Docker Compose:

```bash
# Start the entire infrastructure including monitoring
cd components/infra
make up

# Start only the monitoring components
cd components/infra
make monitoring-up
```

### Configuration Files

The monitoring configuration consists of:

1. Docker Compose configuration in `components/infra/docker-compose.yml`
2. Grafana configuration in `components/infra/grafana/`
3. OpenTelemetry Collector configuration

### Environment Variables

The monitoring stack uses the following environment variables:

| Variable | Description | Default Value |
|----------|-------------|---------------|
| `OTEL_LGTM_ADMIN_USER` | Grafana admin username | admin |
| `OTEL_LGTM_ADMIN_PASSWORD` | Grafana admin password | admin |
| `OTEL_LGTM_EXTERNAL_PORT` | External port for Grafana UI | 3000 |
| `OTEL_LGTM_INTERNAL_PORT` | Internal port for Grafana | 3000 |
| `OTEL_LGTM_RECEIVER_GRPC_PORT` | OTLP GRPC receiver port | 4317 |
| `OTEL_LGTM_RECEIVER_HTTP_PORT` | OTLP HTTP receiver port | 4318 |

### Instrumenting Services

To enable monitoring in application services, configure the following environment variables:

```
# Required for OpenTelemetry instrumentation
OTEL_RESOURCE_SERVICE_NAME=<service-name>
OTEL_LIBRARY_NAME=<library-name>
OTEL_RESOURCE_SERVICE_VERSION=<version>
OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT=<environment>
OTEL_EXPORTER_OTLP_ENDPOINT=http://midaz-otel-lgtm:4317
ENABLE_TELEMETRY=true
```

## Using the Monitoring Tools

### Accessing Grafana

The Grafana UI is accessible at:

```
http://localhost:<OTEL_LGTM_EXTERNAL_PORT>
```

Default credentials are specified in your environment variables (`OTEL_LGTM_ADMIN_USER/OTEL_LGTM_ADMIN_PASSWORD`).

### Exploring Metrics

1. Navigate to the Explore tab in Grafana
2. Select the appropriate data source (Mimir for metrics)
3. Use the query builder or PromQL to query metrics

Example PromQL queries:

```
# Request rate by service
sum(rate(http_server_requests_total[5m])) by (service)

# Error rate
sum(rate(http_server_requests_total{status_code=~"5.."}[5m])) by (service) / sum(rate(http_server_requests_total[5m])) by (service)

# Database query time
histogram_quantile(0.95, sum(rate(database_query_seconds_bucket[5m])) by (operation, le))
```

### Viewing Logs

1. Navigate to the Explore tab in Grafana
2. Select Loki as the data source
3. Use LogQL to query logs

Example LogQL queries:

```
# Logs from a specific service
{service="transaction-service"}

# Error logs
{level="ERROR"}

# Logs related to a specific transaction
{transaction_id="abc123"}
```

### Analyzing Traces

1. Navigate to the Explore tab in Grafana
2. Select Tempo as the data source
3. Search traces by:
   - Trace ID
   - Service name
   - Operation name
   - Duration
   - Tags

### Working with Dashboards

- Use the dashboard selector to navigate between dashboards
- Use dashboard variables to filter by service, environment, etc.
- Set time ranges to focus on specific time periods
- Use dashboard annotations to mark significant events

## Best Practices

### Monitoring Practices

1. **Follow the RED Method** for service monitoring:
   - **R**ate: Requests per second
   - **E**rror rate: Failed requests per second
   - **D**uration: Distribution of request latencies

2. **Follow the USE Method** for resource monitoring:
   - **U**tilization: Percent time the resource is busy
   - **S**aturation: Amount of work resource has to do
   - **E**rrors: Count of error events

3. **Monitor the Four Golden Signals**:
   - Latency
   - Traffic
   - Errors
   - Saturation

### Instrumentation Practices

1. **Add Context to Logs**: Include relevant business context in logs
2. **Use Appropriate Log Levels**: Reserve ERROR for actual failures
3. **Add Custom Metrics**: Instrument business-specific metrics
4. **Correlate with Trace IDs**: Include trace IDs in logs
5. **Use Consistent Naming**: Follow a consistent naming convention for metrics

### Dashboard Practices

1. **Organize by Service**: Group dashboards by service
2. **Start with Overview**: Create overview dashboards for quick assessment
3. **Include Context**: Add documentation to dashboards
4. **Use Variables**: Make dashboards reusable with variables
5. **Set Appropriate Thresholds**: Base alert thresholds on historical data

## Troubleshooting

### Common Issues

#### Monitoring Service Not Available

**Symptoms**: Cannot access Grafana or metrics are missing

**Resolution**:
```bash
# Check service status
cd components/infra
make ps

# Restart monitoring services
cd components/infra
make monitoring-restart
```

#### Missing Metrics

**Symptoms**: Expected metrics are not showing up in Grafana

**Resolution**:
1. Verify service instrumentation is correctly configured
2. Check OpenTelemetry Collector logs:
   ```bash
   cd components/infra
   make logs midaz-otel-lgtm
   ```
3. Verify environment variables are properly set

#### High Cardinality Issues

**Symptoms**: Slow queries in Grafana or "too many time series" errors

**Resolution**:
1. Review metrics labels to reduce cardinality
2. Use label aggregation in queries
3. Adjust retention policies

#### Alert Storm

**Symptoms**: Too many alerts firing simultaneously

**Resolution**:
1. Review alert thresholds and adjust if needed
2. Implement alert grouping
3. Add alert inhibition rules for related alerts

## References

- [Grafana Documentation](https://grafana.com/docs/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [PromQL Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
- [LogQL Query Examples](https://grafana.com/docs/loki/latest/logql/)
- [RED Method](https://www.weave.works/blog/the-red-method-key-metrics-for-microservices-architecture/)
- [USE Method](http://www.brendangregg.com/usemethod.html)