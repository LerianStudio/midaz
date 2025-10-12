# Project Structure Overview

The Midaz repository is organized around two backend services (Onboarding and Transaction), shared libraries under `pkg`, infrastructure assets, and test harnesses. The outline below reflects the current runtime footprint.

#### Directory Layout

```
MIDAZ
 ├── components
 │   ├── console              # Web console (React/Tailwind)
 │   ├── infra                # Docker-compose stacks, infra assets
 │   ├── onboarding           # Onboarding service
 │   │   ├── api              # Generated Swagger docs
 │   │   ├── cmd/app          # Service entrypoint
 │   │   └── internal
 │   │       ├── adapters     # HTTP, database, cache, messaging adapters
 │   │       ├── bootstrap    # Wiring of transport + services
 │   │       └── services     # Command & query use-cases
 │   └── transaction          # Transaction service
 │       ├── api
 │       ├── cmd/app
 │       └── internal (adapters | bootstrap | services)
 ├── image                    # Design assets & documentation
 ├── pkg                      # Shared Go libraries (constants, models, utils, HTTP helpers)
 ├── postman                  # API collections and Newman flows
 ├── reports                  # Generated compliance/test reports
 ├── scripts                  # Automation scripts (tests, tooling)
 └── tests                    # Cross-cutting Go test suites (chaos, property, integration, etc.)
```

#### Layered Architecture (Innermost → Outermost)

1. **Core Entities & Utilities**  
   Packages without internal dependencies: `pkg/constant`, `pkg/mmodel`, `pkg/utils`, `pkg/ptr`, `pkg/gold/parser`, plus generated API specs (`components/onboarding/api`, `components/transaction/api`) and cache bindings (`components/onboarding/internal/adapters/redis`). Test harnesses (`tests/*`, `scripts/analyze_accepted`) also live here as leaf packages.

2. **Shared Foundations**  
   Cross-cutting building blocks that aggregate core types: `pkg`, `pkg/gold/transaction`, and RabbitMQ producers in both services (`components/*/internal/adapters/rabbitmq`).

3. **Service Facades**  
   Common service glue that depends on shared foundations but not concrete storage: `components/onboarding/internal/services`, `components/transaction/internal/services`, `components/transaction/internal/adapters/redis`, and transport helpers in `pkg/net/http`.

4. **Infrastructure Adapters**  
   Database integrations for MongoDB and PostgreSQL across both services (`components/*/internal/adapters/{mongodb,postgres/...}`).

5. **Use-Case Orchestrators**  
   Command/query orchestration and transactional aggregates (`components/onboarding/internal/services/{command,query}`, `components/transaction/internal/adapters/postgres/{transaction,transactionroute}`).

6. **Transport Entry**  
   HTTP routing and service exposure (`components/onboarding/internal/adapters/http/in`, `components/transaction/internal/services/{command,query}`, `components/transaction/internal/adapters/http/in`).

7. **Bootstrap & Launch Wiring**  
   Dependency orchestration that ties adapters to executables (`components/onboarding/internal/bootstrap`, `components/transaction/internal/bootstrap`).

8. **Service Entrypoints**  
   Go binaries that start each service (`components/onboarding/cmd/app`, `components/transaction/cmd/app`).

#### Shared Libraries (`./pkg`)

| Directory         | Purpose                                                                    |
|-------------------|----------------------------------------------------------------------------|
| `pkg/constant`     | Domain constants shared across services                                   |
| `pkg/mmodel`       | Canonical domain models                                                   |
| `pkg/utils`        | General utilities (cache helpers, jitter, etc.)                          |
| `pkg/ptr`          | Lightweight helpers for pointer creation (e.g., `StringPtr`)             |
| `pkg/gold`         | Gold language parser and transaction helpers                             |
| `pkg/net/http`     | HTTP client helpers and error translation                                |
| `pkg`              | Common error types and generic helpers used by both services             |

#### Components (`./components`)

- **Onboarding Service**  
  Responsible for organization onboarding flows. Internal adapters interact with MongoDB/PostgreSQL, Redis, and RabbitMQ. Command/query layers coordinate repositories before the HTTP surface exposes functionality.

- **Transaction Service**  
  Manages ledger transactions, balances, operations, and routes. Mirrors the onboarding layering with its own adapters and orchestrators.

- **Console**  
  React application for administrative operations. Lives outside the Go layering but shares the same component directory.

- **Infra**  
  Docker Compose stacks, Grafana dashboards, and other infrastructure artifacts for local and CI environments.

#### Tests & Tooling

- `tests/` holds Go-based chaos, fuzzy, property, and integration suites that exercise the services end-to-end.
- `scripts/` centralizes automation (e.g., `run-tests.sh`, log checks, documentation tooling).
- `postman/` provides API collections and Newman automation for workflow validation.

The repository centers on the two backend services and shared libraries while retaining supporting tooling and UI assets.
