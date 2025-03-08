# Midaz Grafana Dashboards

This directory contains Grafana dashboard definitions for monitoring the Midaz application.

## HTTP Monitoring Dashboards

The HTTP monitoring dashboards provide comprehensive visibility into HTTP traffic across Midaz services:

### 1. Midaz HTTP Monitoring Dashboard

**File:** `midaz-http-dashboard.json`

This dashboard provides a high-level overview of HTTP server metrics across all Midaz services:

- HTTP request rates by service
- Response time percentiles (p50, p95)
- Status code distribution
- Error rates (5xx responses)
- Request rates by path
- HTTP error logs

Use this dashboard to monitor the overall health and performance of your HTTP services.

### 2. Midaz HTTP Endpoints Dashboard

**File:** `midaz-http-endpoints-dashboard.json`

This dashboard focuses on endpoint-specific metrics to help identify performance bottlenecks:

- Top 10 endpoints by request rate
- Top 10 slowest endpoints (p95 response time)
- Response time by endpoint
- Error rate by endpoint
- GET vs POST/PUT/DELETE request rates
- Endpoint-specific error logs

Use this dashboard to drill down into specific API endpoints and optimize their performance.

### 3. Midaz HTTP Client Dashboard

**File:** `midaz-http-client-dashboard.json`

This dashboard monitors outgoing HTTP requests from Midaz services to external systems:

- Client request rates by service
- Client response times
- Status code distribution for client requests
- Client error rates
- Request rates by target host
- Connection pool metrics
- Client error logs

Use this dashboard to ensure reliable communication with external services and dependencies.

## Usage

These dashboards are automatically provisioned in Grafana through the configuration in `../provisioning/dashboards/midaz-dashboards.yaml`.

To access these dashboards:

1. Navigate to the Grafana UI (typically at http://localhost:3000)
2. Log in with your credentials
3. Go to Dashboards > Browse
4. Look for the "Midaz" folder
5. Select the desired dashboard

## Customization

You can customize these dashboards directly through the Grafana UI. Changes will be persisted if `allowUiUpdates` is set to `true` in the provisioning configuration.

To add a new dashboard:

1. Create a new JSON file in this directory
2. Follow the Grafana dashboard JSON format
3. Restart Grafana or wait for the configured `updateIntervalSeconds`

## Metrics Requirements

These dashboards expect the following Prometheus metrics to be available:

- `http_server_requests_total` - Counter for incoming HTTP requests
- `http_server_request_duration_seconds` - Histogram for HTTP request duration
- `http_client_requests_total` - Counter for outgoing HTTP requests
- `http_client_request_duration_seconds` - Histogram for HTTP client request duration
- `http_client_in_flight_requests` - Gauge for in-flight HTTP client requests
- `http_client_connection_pool_size` - Gauge for HTTP client connection pool size
- `http_client_connection_pool_idle` - Gauge for idle connections in HTTP client pool

Ensure your services are properly instrumented to expose these metrics. 