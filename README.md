![banner](image/README/midaz-banner.png) 

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/LerianStudio/midaz?include_prereleases)](https://github.com/LerianStudio/midaz/releases)
[![License: Elastic-2.0](https://img.shields.io/badge/License-Elastic_2.0-blue.svg)](https://github.com/LerianStudio/midaz/blob/main/LICENSE)
[![Go Report](https://goreportcard.com/badge/github.com/lerianstudio/midaz)](https://goreportcard.com/report/github.com/lerianstudio/midaz)
[![Discord](https://img.shields.io/badge/Discord-Lerian%20Studio-%237289da.svg?logo=discord)](https://discord.gg/DnhqKwkGv3)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/LerianStudio/midaz)

</div>

# Lerian Midaz: Source-Available Core Banking Platform

Midaz is a composable core banking platform built around a double-entry ledger. One Go monorepo ships the ledger core (onboarding, transactions, CRM, fees), real-time transaction validation and fraud prevention (Tracer), and async reporting (Reporter) — all under the [Elastic License 2.0](LICENSE), source-available. Transactional messaging (PIX, cards, wires) and governance integrations remain external and plug in via the marketplace.

## What's in the Box

- **Ledger core** (one binary): double-entry accounting over the full financial hierarchy — onboarding (organizations, ledgers, assets, portfolios, segments, accounts), n:n transactions, CRM (holders and instruments) with field-level encryption and searchable hashing so PII stays queryable without exposing plaintext, and in-process fee calculation on the transaction path.
- **Tracer**: real-time transaction validation and fraud prevention — a CEL rule engine, multi-scope spending limits, and a hash-chained immutable audit trail for compliance.
- **Reporter**: templated async reporting — a manager API queues jobs onto RabbitMQ and a headless worker renders HTML, PDF, CSV, XML, and TXT, storing artifacts in S3-compatible object storage.

## Quickstart

Prerequisites: Go 1.26.4+ and Docker.

```bash
git clone https://github.com/LerianStudio/midaz.git
cd midaz
make set-env   # create .env files from .env.example
make up        # start infra, then all services
```

Services after `make up`:

| Service | URL |
| --- | --- |
| Ledger API | http://localhost:3002 |
| Tracer | http://localhost:4020 |

Full guide, API references, and best practices: [docs.lerian.studio](https://docs.lerian.studio).

## Architecture

Both Go units build from the single root module (`github.com/LerianStudio/midaz/v4`) and ship under a single unified version.

| Unit | Port | Role | Stores |
| --- | --- | --- | --- |
| Ledger | :3002 | Unified binary: onboarding + transaction + CRM + fees, in-process | PostgreSQL, MongoDB |
| Tracer | :4020 | Real-time validation/fraud: CEL rules, spending limits, hash-chained audit | PostgreSQL |
| Infra | — | docker-compose stack | — |

Infrastructure: PostgreSQL 17 (primary/replica), MongoDB replica set, RabbitMQ, Valkey, and Grafana/OpenTelemetry.

The ledger reaches Tracer over an opt-in reservation seam, enabled by setting `TRACER_BASE_URL`. The transport is gRPC by default with a selectable REST fallback (`TRACER_TRANSPORT`), authenticated by mutual TLS (`TRACER_TLS_MODE=mtls`) or delegated to a service-mesh sidecar (`mesh`), forwarding a trusted `x-tenant-id` for per-tenant pool resolution. See [docs/architecture/ledger-tracer-topology.md](docs/architecture/ledger-tracer-topology.md).

CRM routes register under the `midaz` authorization namespace; the coordinated tenant-manager RBAC policy migration is the X1 release gate (see [docs/auth/RBAC-NAMESPACES.md](docs/auth/RBAC-NAMESPACES.md)).

### Domain Hierarchy

- **Organizations** — top-level entities, optionally with parent-child relationships.
- **Ledgers** — financial record-keeping systems belonging to organizations.
- **Assets** — types of value (currencies, securities) with specific codes.
- **Portfolios** — collections of accounts for organizational purposes.
- **Segments** — categories for grouping accounts (e.g. by department or product line).
- **Accounts** — basic units for tracking financial resources, linked to assets.
- **Transactions** — debits and credits, balanced by double-entry rules.
- **Balances** — available and on-hold amounts tracked per account.

### Key Capabilities

- **Double-entry engine** — every credit has a matching debit.
- **Multi-asset support** — transactions across currencies with automatic rate conversion.
- **Complex transactions** — n:n operations (multiple sources to multiple destinations).
- **Gold transaction DSL** — a purpose-built grammar for modeling complex transactions.
- **Immutable records** — every transaction is permanently recorded for audit.
- **Async processing** — event-driven transaction handling via RabbitMQ.
- **Optimistic-concurrency balances** — version-based concurrency control for balance updates.
- **Hexagonal + CQRS** — domain logic isolated from adapters; commands and queries separated.
- **OpenAPI docs** — RESTful endpoints with generated OpenAPI specifications.

## Community & Support

- Join our [Discord community](https://discord.gg/DnhqKwkGv3) for discussions, support, and updates.
- For bug reports and feature requests, please use our [GitHub Issues](https://github.com/LerianStudio/midaz/issues).
- If you want to raise anything to the attention of the community, open a Discussion in our [GitHub](https://github.com/LerianStudio/midaz/discussions).
- Follow us on [Twitter](https://twitter.com/LerianStudio) for the latest news and announcements.

## Repo Activity

![Alt](https://repobeats.axiom.co/api/embed/827f95068c3eb21900ed6a7191a53639481cbc75.svg "Repobeats analytics image")

## Contributing & License

We welcome contributions from the community. Read our [Contributing Guidelines](CONTRIBUTING.md) to get started. Lerian Midaz is released under the [Elastic License 2.0 (ELv2)](LICENSE) — a source-available license that allows you to use, copy, modify, and redistribute Midaz, with three primary limitations: you may not provide it to others as a managed/hosted service, circumvent its license key functionality, or remove/obscure license notices.

## About Lerian

Midaz is developed by Lerian, a tech company founded in 2024. Our team has a track record in developing ledger and core banking solutions. For inquiries or support, reach out at [contact@lerian.studio](mailto:contact@lerian.studio) or open a Discussion in our [GitHub repository](https://github.com/LerianStudio/midaz/discussions).
