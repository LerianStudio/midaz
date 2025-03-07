# Grafana Dashboards for Midaz

This directory contains the Grafana dashboards and configuration for the Midaz application.

## Dashboards

- **Midaz Application Logs**: Displays logs from the onboarding and transaction components, with filtering by service, log level, and other attributes.

## Automated Setup

The dashboards and datasources are automatically provisioned when the OTEL LGTM container starts. The process works as follows:

1. The `run-grafana.sh` script starts Grafana and then runs the `init-grafana.sh` script.
2. The `init-grafana.sh` script waits for Grafana to be ready, then creates the datasources and dashboards.
3. The dashboards are loaded from the `dashboards` directory.
4. The datasources are configured from the `provisioning/datasources` directory.

## Manual Access

You can access Grafana at http://localhost:3100 with the following credentials:

- Username: midaz
- Password: lerian

## Adding New Dashboards

To add a new dashboard:

1. Create a new JSON file in the `dashboards` directory.
2. Restart the OTEL LGTM container using the `make telemetry-restart` command.

## Troubleshooting

If the dashboards are not appearing:

1. Check that Grafana is running: `docker ps | grep midaz-otel-lgtm`
2. Check the Grafana logs: `docker logs midaz-otel-lgtm | grep -i grafana`
3. Verify that the datasources are configured: `curl -u midaz:lerian http://localhost:3100/api/datasources`
4. Restart the OTEL LGTM container: `make telemetry-restart` 