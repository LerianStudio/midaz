# Repository Guidelines

Midaz is an open-source financial ledger focused on fast onboarding and high-throughput transaction processing. The system follows a CQRS-inspired layering: onboarding handles master data (organizations, portfolios, accounts) while transaction ingests and routes ledger activity. Shared Go libraries in `pkg/` provide canonical models, constants, and protocol helpers that keep both services aligned.

## Project Overview & Structure
- `components/onboarding` and `components/transaction` are the primary Go services. Each follows `internal/adapters → internal/services → internal/bootstrap`, exposing binaries under `cmd/app`.
- `components/console` delivers the operational React UI; `components/infra` contains Docker stacks, Grafana dashboards, and provisioning assets.
- Service-agnostic logic resides in `pkg/` (models, utilities, Gold DSL parser, HTTP helpers). Go suites live in `tests/`, Newman collections in `postman/`, automation scripts in `scripts/`, and generated Swagger artifacts under `components/*/api`.

## Build, Test, and Development Commands
- `make build` — orchestrates Go builds and console bundling.
- `make test` — runs unit tests component-by-component; for a full sweep use `scripts/run-tests.sh`.
- `make lint` — installs and executes `golangci-lint` plus repository-wide lint configs.
- `make format` — delegates to component formatters (`gofmt`, Prettier, etc.).
- Spot checks: `go test ./pkg/...`, `go test ./components/onboarding/internal/services/...`, or `npm test` inside `components/console`.

## Coding Style & Naming Conventions
- Go code must pass `gofmt`; exported identifiers use `CamelCase`, packages remain lowercase, and JSON tags follow `snake_case`. Adhere to `revive.toml` and `golangci-lint` defaults.
- Shared pointer helpers now live in `pkg/ptr`; reuse them instead of reintroducing CLI artifacts.
- Console code follows ESLint/Prettier standards committed in that package.

## Testing Guidelines
- Write table-driven Go tests with `testing`, `stretchr/testify`, and `go.uber.org/mock`. Mirror directory structure (e.g., `internal/services/command/create_account_test.go`).
- Integration, chaos, fuzzy, and property suites live in `tests/`; configure required endpoints via `.env.example` templates.
- Keep `go test ./...` green before pushing. Document any skips or flaky cases directly in the PR.

## Commit & Pull Request Guidelines
- Follow Conventional Commits (`feat:`, `fix:`, `chore:`) from `CHANGELOG.md`. Keep subject lines imperative and <72 characters.
- Scope commits narrowly (e.g., shared library updates separated from service code).
- PRs should include a summary, linked issue, test evidence (commands/logs), and screenshots for console changes. Call out schema, migration, or infra alterations with rollout steps.

## Security & Configuration Notes
- Secrets are sourced via service-specific `.env` files; never commit real credentials. Use `make set-env` when onboarding.
- RabbitMQ, Redis/Valkey, and Postgres defaults live under `components/infra`; update compose files and charts beside code changes.
