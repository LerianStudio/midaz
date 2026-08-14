# Midaz Test Suites

This directory hosts black-box test suites for the Midaz stack when running locally via Docker Compose. The tests target the HTTP API of the unified `ledger` binary (onboarding + transaction + CRM + fees on `:3002`) and exercise the system end-to-end against the local infrastructure (PostgreSQL, MongoDB, Valkey, RabbitMQ, OTEL LGTM) brought up from `components/infra`. The repo is a single Go module (`github.com/LerianStudio/midaz/v4`, one root `go.mod`); these suites compile against it directly.

Layout:

- `chaos/`: Fault-injection around containers (stop/pause/restart) while verifying system behavior.
- `helpers/`: Shared Go helpers (env/URL resolution, HTTP, auth, balances, docker, chaos).
- `utils/`: Shared test utilities (chaos/mongodb/postgres/rabbitmq/redis/stubs + crypto/helpers).
- `reporter/`: Reporter subsystem suites (`integration`, `e2e`, `property`, `fuzzy`, `chaos`), each gated by its own build tag (`integration`, `e2e`, `property`, `fuzz`, `chaos`) and built on the testcontainers harness in `pkg/reporter/itestkit`. See `tests/reporter/README.md`.

Prerequisites:

- Docker and Docker Compose available in PATH.
- `make up` to bring infra (`components/infra`) and the Go service components online, or allow tests to manage the stack via env flags.
- Default ports come from `tests/helpers` and can be overridden via env vars.

Environment variables:

- ONBOARDING_URL: Base URL for the onboarding API surface (default `http://localhost:3002`).
- TRANSACTION_URL: Base URL for the transaction API surface (default `http://localhost:3002`).
- MIDAZ_TEST_MANAGE_STACK: If `true`, tests may start/stop the stack themselves.

Running:

- Chaos tests: `go test -v ./tests/chaos`.
- Reporter suites: build-tag gated, e.g. `go test -tags integration -v ./tests/reporter/integration` (use the matching tag per suite: `integration`, `e2e`, `property`, `fuzz`, `chaos`). See `tests/reporter/README.md`.

Notes:

- See `tests/helpers` and `tests/utils` for shared utilities.

