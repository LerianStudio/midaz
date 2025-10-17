# Midaz Test Suites

This directory hosts black-box test suites for the Midaz stack when running locally via Docker Compose. The tests target the HTTP APIs of the onboarding and transaction services and exercise the system end-to-end against the local infrastructure (PostgreSQL, MongoDB, Valkey, RabbitMQ, OTEL LGTM).

Suites:

- integration: Validates service interactions across boundaries and persistence.
- e2e: Covers the full happy-path workflow (organization → ledger → account → transactions).
- property: Property-based checks for invariants (e.g., balances never negative when rules apply).
- fuzzy: Fuzz inputs to validate robustness and input validation.
- chaos: Fault-injection around containers (stop/pause/restart) while verifying system behavior.

Prerequisites:

- Docker and Docker Compose available in PATH.
- `make up-backend` to bring infra + onboarding + transaction online, or allow tests to manage the stack via env flags.
- Default ports: onboarding `http://localhost:3000`, transaction `http://localhost:3001`. These can be overridden via env vars.

Environment variables:

- ONBOARDING_URL: Base URL for onboarding API (default `http://localhost:3000`).
- TRANSACTION_URL: Base URL for transaction API (default `http://localhost:3001`).
- MIDAZ_TEST_MANAGE_STACK: If `true`, tests may start/stop the stack using `make up-backend`/`make down-backend`.

Running:

- All integration tests: `go test -v ./tests/integration`.
- E2E tests: `go test -v ./tests/e2e`.
- Property tests: `go test -v ./tests/property`.
- Fuzz targets (examples): `go test -v ./tests/fuzzy -fuzz=Fuzz -run=^$ -fuzztime=5s`.
- Chaos tests: `go test -v ./tests/chaos`.

Notes:

- Many initial tests are placeholders with `t.Skip` to stage the structure; they will be implemented iteratively.
- See `tests/helpers` and `tests/fixtures` for shared utilities and sample payloads.

