# Reporter Manager

Reporter Manager is the REST API for Midaz's reporting subsystem. It manages report templates, reports, and delivery deadlines, and dispatches report-generation jobs onto RabbitMQ for the Reporter Worker to render asynchronously. It exposes a Swagger-documented HTTP API on port `4005`.

## How It Fits

- **Monorepo deploy unit.** Co-located in the Midaz monorepo under a single root `go.mod` (module `github.com/LerianStudio/midaz/v4`, Go 1.26.3 / toolchain go1.26.4). It builds from `components/reporter-manager/cmd/app/main.go`; it ships no own `go.mod`.
- **Producer in a producer/consumer pair.** Manager is the producer; [`reporter-worker`](../reporter-worker/) is the consumer. They share infrastructure: the same RabbitMQ topology (`reporter.generate-report.*`), the same MongoDB database (`reporter-db`), and the same S3-compatible object store (`reporter-storage`).
- **Ports.** REST API on `SERVER_PORT=4005`. The production image is `gcr.io/distroless/static-debian12:nonroot` (no shell), so health is probed externally by the orchestrator against `/health`.
- **Shared infra.** PostgreSQL, MongoDB, RabbitMQ, and the SeaweedFS object store come from [`components/infra`](../infra/). This component's `docker-compose.yml` carries only the `reporter-manager` app service and joins the external `infra-network`.

## Key Behaviors

- CRUD for report templates (including a block-based template builder), reports, and delivery deadlines.
- On report creation, enqueues a generation job to the RabbitMQ exchange `reporter.generate-report.exchange` (queue `reporter.generate-report.queue`, routing key `reporter.generate-report.key`).
- Persists template/report/deadline state to the MongoDB `reporter-db` database.
- Reads ledger/onboarding data either via read-only Postgres queries (`FETCHER_ENABLED=false`, single-tenant only) or via an external Fetcher service (`FETCHER_ENABLED=true`). Multi-tenant mode requires Fetcher mode.

## Configuration

All configuration is via environment variables; copy `.env.example` to `.env` and adjust. Key variables:

| Variable | Purpose |
|----------|---------|
| `SERVER_PORT` | REST API port (default `4005`). |
| `MONGO_NAME` | MongoDB database (default `reporter-db`). |
| `RABBITMQ_EXCHANGE` / `RABBITMQ_GENERATE_REPORT_QUEUE` / `RABBITMQ_GENERATE_REPORT_KEY` | Generation-job topology shared with the worker. |
| `OBJECT_STORAGE_ENDPOINT` / `OBJECT_STORAGE_BUCKET` | S3-compatible store for report artifacts (default bucket `reporter-storage`). |
| `FETCHER_ENABLED` / `FETCHER_URL` | Toggle direct-Postgres vs. Fetcher data sourcing. |
| `MULTI_TENANT_ENABLED` | Per-tenant isolation; requires `FETCHER_ENABLED=true`. |

## Running Locally

```bash
make set-env   # copy .env.example -> .env
make up        # docker compose up -d (joins the shared infra-network)
make down      # stop and remove containers
make logs      # tail service logs
```

The shared infrastructure must be running first (see [`components/infra`](../infra/)). From the monorepo root, the component can be driven via `make reporter-manager COMMAND=<target>`.

```bash
make build           # build the binary into ./.bin
make test            # run tests
make lint            # run golangci-lint
make generate-docs   # regenerate Swagger docs into ./api
```

The Swagger spec declares host `localhost:4005`.

## Tests and Shared Library

- Test suites (unit, integration, property, fuzzy, chaos): [`tests/reporter`](../../tests/reporter/).
- Shared reporter library (datasource config, readyz checkers, circuit breaker, backoff, PDF pool): [`pkg/reporter`](../../pkg/reporter/).
