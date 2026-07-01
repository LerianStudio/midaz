# Infra

> Shared local infrastructure for the Midaz stack — no Go build

[![License](https://img.shields.io/badge/license-Elastic%20License%202.0-4c1.svg)](../../LICENSE)

Infra is the infrastructure-only component of the [Midaz monorepo](https://github.com/LerianStudio/midaz).
It ships no application binary — just a consolidated Docker Compose stack and the shared
`infra-network` that the Go components (`ledger` on :3002, `tracer` on :4020) attach to.

---

## What it provisions

[`docker-compose.yml`](docker-compose.yml) declares the shared backing services:

| Service | Image | Purpose |
|---------|-------|---------|
| `midaz-postgres-primary` | `postgres:17` | Primary relational store (onboarding + transaction) |
| `midaz-postgres-replica` | `postgres:17` | Streaming read replica |
| `midaz-mongodb` (+ `-init`) | `mongo:8` | Metadata, CRM, and fee documents (replica set `rs0`) |
| `midaz-valkey` | `valkey/valkey:8` | Cache and balance-sync (Redis-compatible) |
| `midaz-rabbitmq` | `rabbitmq:4.1.3-management-alpine` | Async transaction processing |
| `midaz-otel-lgtm` | `grafana/otel-lgtm:latest` | Grafana + Loki/Tempo/Prometheus + OTLP collector |
| `midaz-hc-vault` (+ `-init`) | `hashicorp/vault:1.15` | **Optional** — Transit engine for CRM envelope encryption |

Vault is opt-in and not started by default. The init service enables the mode-derived Transit engine
(`transit-st` single-tenant / `transit-mt` multi-tenant); the ledger selects it via `KMS_VENDOR=hashicorp-vault`.

---

## Start / stop

```bash
# From the repo root — orchestrates infra first, then the Go components:
make up
make down

# Just this component's services:
make -C components/infra up
make -C components/infra down
```

Docker targets (`up`, `down`, `start`, `stop`, `restart`, `logs`) come from the shared `mk/docker.mk`
fragment included by the component [`Makefile`](Makefile); this component's own targets are no-op
build/clean plus the Vault Transit helpers (`vault-transit-single`, `vault-transit-multi`,
`vault-transit-setup`, `vault-transit-list`, `vault-transit-disable`).

---

## Network & ports

All services join the external bridge network **`infra-network`** (`name: infra-network`), which the
`ledger` and `tracer` compose files attach to. Host ports are configured in
[`.env.example`](.env.example):

| Service | Host port(s) |
|---------|--------------|
| PostgreSQL primary / replica | `5701` / `5702` |
| MongoDB | `5703` |
| Valkey | `5704` |
| RabbitMQ (mgmt / AMQP) | `3003` / `3004` |
| Grafana / otel-lgtm | `3100` (UI), `4317` (OTLP gRPC), `4318` (OTLP HTTP) |
| Vault (optional) | `8200` |

Copy `.env.example` to `.env` (or run `make set-env` at the repo root) before starting.

---

## Documentation

For the bigger picture — architecture, coding rules, and the components that consume this
infrastructure — see the repo-root [`CLAUDE.md`](../../CLAUDE.md), [`AGENTS.md`](../../AGENTS.md),
and [`docs/PROJECT_RULES.md`](../../docs/PROJECT_RULES.md).

---

## License

Elastic License 2.0, source-available. See the [LICENSE](../../LICENSE) file.
