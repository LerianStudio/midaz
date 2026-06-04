# Reporter Worker

Reporter Worker is the asynchronous report generator for Midaz's reporting subsystem. It is a RabbitMQ consumer, not a REST service: it pulls generation jobs off the queue, renders reports (HTML, CSV, JSON), optionally converts them to PDF, and stores the artifacts in S3-compatible object storage. Its only HTTP surface is a bare `net/http` health server.

## How It Fits

- **Monorepo deploy unit.** Co-located in the Midaz monorepo under a single root `go.mod` (module `github.com/LerianStudio/midaz/v3`, Go 1.26.3 / toolchain go1.26.4). It builds from `components/reporter-worker/cmd/app/main.go`; it ships no own `go.mod`.
- **Consumer in a producer/consumer pair.** Worker is the consumer; [`reporter-manager`](../reporter-manager/) is the producer. They share infrastructure: the same RabbitMQ topology (`reporter.generate-report.*`), the same MongoDB database (`reporter-db`), and the same S3-compatible object store (`reporter-storage`).
- **Ports.** No REST API. A stdlib `net/http` health server exposes `/health` (liveness) and `/readyz` (readiness) on `HEALTH_PORT=4006`. Do not confuse this with the Fetcher service, whose default port is also `4006` on a different host.
- **Runtime image.** `alpine:3.23` with system Chromium installed: the worker renders PDFs via `chromedp` driving headless Chromium, which the distroless base cannot host. The image carries an embedded Docker `HEALTHCHECK` against `/health`.
- **Shared infra.** PostgreSQL, MongoDB, RabbitMQ, and the SeaweedFS object store come from [`components/infra`](../infra/). This component's `docker-compose.yml` carries only the `reporter-worker` app service and joins the external `infra-network`.

## Key Behaviors

- Consumes generation jobs from the RabbitMQ queue `reporter.generate-report.queue` (exchange `reporter.generate-report.exchange`, routing key `reporter.generate-report.key`); failures route to the dead-letter queue `reporter.dlq`.
- Renders report output as HTML, CSV, or JSON; when the job requests PDF, converts the rendered HTML via a pooled `chromedp` worker set.
- Uploads finished artifacts to the S3-compatible store (bucket `reporter-storage`).
- Reads ledger/onboarding data either via read-only Postgres queries (`FETCHER_ENABLED=false`, single-tenant only) or via an external Fetcher service (`FETCHER_ENABLED=true`). Multi-tenant mode requires Fetcher mode.

## Configuration

All configuration is via environment variables; copy `.env.example` to `.env` and adjust. Key variables:

| Variable | Purpose |
|----------|---------|
| `HEALTH_PORT` | Health-server port for `/health` and `/readyz` (default `4006`). |
| `RABBITMQ_NUMBERS_OF_WORKERS` | Concurrent RabbitMQ consumers. |
| `RABBITMQ_DLQ_QUEUE` | Dead-letter queue for failed jobs (default `reporter.dlq`). |
| `PDF_POOL_WORKERS` / `PDF_TIMEOUT_SECONDS` | Chromium render-pool size and per-render timeout. |
| `OBJECT_STORAGE_ENDPOINT` / `OBJECT_STORAGE_BUCKET` | S3-compatible artifact store (default bucket `reporter-storage`). |
| `FETCHER_ENABLED` / `MULTI_TENANT_ENABLED` | Data-sourcing mode; multi-tenant requires Fetcher mode. |

## Running Locally

```bash
make set-env   # copy .env.example -> .env
make up        # docker compose up -d (joins the shared infra-network)
make down      # stop and remove containers
make logs      # tail service logs
```

The shared infrastructure must be running first (see [`components/infra`](../infra/)). From the monorepo root, the component can be driven via `make reporter-worker COMMAND=<target>`.

```bash
make build              # build the binary into ./.bin
make test               # run tests
make test-multi-tenant  # run multi-tenant tagged tests
make lint               # run golangci-lint
```

## Tests and Shared Library

- Test suites (unit, integration, property, fuzzy, chaos): [`tests/reporter`](../../tests/reporter/).
- Shared reporter library (datasource config, readyz checkers, circuit breaker, backoff, PDF pool): [`pkg/reporter`](../../pkg/reporter/).
