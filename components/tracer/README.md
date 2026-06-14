# Tracer

> Real-time transaction validation and fraud prevention API for financial systems

[![Go Version](https://img.shields.io/badge/Go-1.26.4+-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-336791?style=flat&logo=postgresql)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-Elastic%20License%202.0-4c1.svg)](LICENSE)

Tracer is a co-located deploy unit in the [Midaz monorepo](https://github.com/LerianStudio/midaz),
released on the single unified Midaz version.

---

## 📋 Table of Contents

- [Overview](#overview)
- [Core Concepts](#core-concepts)
- [How It Works](#how-it-works)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Development](#development)
- [API Reference](#api-reference)
- [Testing](#testing)
- [Deployment](#deployment)
- [Documentation](#documentation)

---

## Overview

**Tracer** is a product-agnostic validation service that provides instant decisions (ALLOW/DENY/REVIEW) for financial transactions using sophisticated rule expressions and flexible spending limits.

### Why Tracer?

- **Independent validation layer** - Decouples fraud prevention from business logic
- **Sub-100ms decisions** - Non-blocking, real-time validation (<80ms p99)
- **Flexible rule engine** - Type-safe expressions using CEL (Common Expression Language)
- **Multi-scope limits** - Apply spending controls at account, segment, or portfolio level
- **Audit-ready** - Complete transaction history with SOX/GLBA compliance (7-year retention)

### Use Cases

- Payment authorization and fraud detection
- Transfer limits and velocity checks
- Withdrawal controls and risk scoring
- Multi-product transaction validation
- Regulatory compliance monitoring

---

## Core Concepts

### 1. **Validation Request**

Every transaction submitted to Tracer contains:
- **Request ID** - Unique identifier for idempotency
- **Transaction data** - Type (CARD/WIRE/PIX/CRYPTO), amount (decimal), currency, timestamp
- **Account context** - Account ID, type, status (required)
- **Optional contexts** - Segment, portfolio, merchant information
- **Metadata** - Custom key-value pairs for business rules

### 2. **Rules Engine**

Rules are CEL expressions evaluated against transaction data:

```cel
// Example: Deny high-value transactions (amount in decimal)
amount > 10000

// Example: Review transactions for premium merchants
size(merchant) > 0 &&
merchant["category"] == "5411" &&
amount > 5000
```

### 3. **Spending Limits**

Hierarchical limits with configurable scopes:
- **Account-level**: Per-user limits (e.g., $5,000/day)
- **Segment-level**: Group limits (e.g., VIP users: $50,000/day)
- **Portfolio-level**: Organization-wide caps (e.g., $1M/day)

### 4. **Decision Flow**

```text
Transaction → Rules Evaluation → Limit Check → Decision
                ↓                   ↓            ↓
              DENY              EXCEEDED      ALLOW/DENY/REVIEW
```

### 5. **Audit Trail**

Every validation creates an immutable audit record:
- Request/response payloads
- Rule evaluation results
- Decision rationale
- Timestamp and correlation ID

---

## How It Works

### Validation Flow

```text
┌─────────────┐
│   Client    │
│  (Payment)  │
└──────┬──────┘
       │ POST /v1/validations
       ▼
┌─────────────────────────────────────────────────────┐
│                    Tracer API                       │
├─────────────────────────────────────────────────────┤
│  1. Authentication (API Key)                        │
│  2. Input Validation                                │
│  3. Rule Evaluation (CEL Engine)                    │
│     ├─ Fetch active rules for transaction type      │
│     ├─ Execute CEL expressions                      │
│     └─ Determine rule-based decision                │
│  4. Limit Check (Spending Limits)                   │
│     ├─ Identify applicable limits (scope)           │
│     ├─ Calculate current usage                      │
│     └─ Verify limit compliance                      │
│  5. Final Decision (ALLOW/DENY/REVIEW)              │
│  6. Audit Log (Immutable Record)                    │
└──────┬──────────────────────────────────────────────┘
       │
       ▼
┌─────────────┐
│  Response   │
│  {          │
│   decision, │
│   reason,   │
│   metadata  │
│  }          │
└─────────────┘
```

### Decision Logic

1. **DENY** - Rule violation or limit exceeded → Transaction rejected
2. **REVIEW** - Suspicious activity detected → Manual review required
3. **ALLOW** - All checks passed → Transaction approved

---

## Features

### 🚀 Performance

- **Target sub-100ms validation** - Designed for P99 latency <80ms on typical payloads (simple rules, <2KB requests)
- **Concurrent processing** - Target throughput of ~1000 req/s per instance under normal load conditions
- **In-memory rule cache** - Hot path optimization for frequent evaluations

### 🔧 Rule Engine

- **CEL expressions** - Type-safe, sandboxed logic with Google's CEL
- **Dynamic rules** - Add/update rules without deployment
- **Priority-based execution** - Control evaluation order
- **Rich context** - Access transaction, user, and historical data

### 💰 Spending Limits

- **Multi-scope application** - Account, segment, and portfolio levels
- **Time-based windows** - Daily, monthly, per-transaction periods
- **Reset strategies** - Rolling window or calendar-based
- **Override capabilities** - Emergency limit adjustments

### 📊 Observability

- **OpenTelemetry** - Distributed tracing with Jaeger
- **Structured logging** - JSON logs with correlation IDs
- **Prometheus metrics** - Request rates, latencies, error rates
- **Health endpoints** - Liveness (`/health`) and readiness (`/readyz`) probes

### 🔐 Security

- **API Key authentication** - Per-organization keys
- **Resource authorization** - Validates access to specific resources
- **Input validation** - Struct tags + validator/v10
- **Audit compliance** - SOX/GLBA 7-year retention

### 🏗️ Architecture

- **Hexagonal Architecture** - Clean separation of concerns (Ports & Adapters)
- **CQRS pattern** - Command/Query segregation for clarity
- **Product-agnostic** - Works with any transaction type
- **Multi-tenant capable** - Tenant-isolated DB resolution, gated by `MULTI_TENANT_ENABLED` (default off)

---

## Architecture

### Pattern: Hexagonal Architecture + CQRS

**Philosophy:** Business logic isolated from infrastructure. Domain services know nothing about HTTP, databases, or frameworks.

```text
┌───────────────────────────────────────────────────────────┐
│                        External                           │
│                    ┌──────────────┐                       │
│                    │  HTTP Client │                       │
│                    └───────┬──────┘                       │
└────────────────────────────┼──────────────────────────────┘
                             │
                    ┌────────▼────────┐
                    │   HTTP Adapter  │  (Fiber handlers)
                    │  /adapters/http │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼─────┐      ┌──────▼──────┐    ┌──────▼──────┐
    │ Command  │      │    Query    │    │  Validator  │
    │ Services │      │  Services   │    │  Services   │
    └────┬─────┘      └──────┬──────┘    └──────┬──────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │
                    ┌────────▼────────┐
                    │  Domain Models  │  (Entities & DTOs)
                    │   /pkg/model    │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼─────┐      ┌──────▼──────┐    ┌──────▼──────┐
    │PostgreSQL│      │ CEL Engine  │    │   Tracer    │
    │ Adapter  │      │  Adapter    │    │  Middleware │
    └──────────┘      └─────────────┘    └─────────────┘
```

### Project Structure

```text
tracer/
├── cmd/
│   └── app/                    # Application entry point
│       └── main.go            # Bootstrap & DI container
│
├── internal/
│   ├── bootstrap/             # Dependency injection setup
│   │   └── config.go          # Environment configuration
│   │
│   ├── services/              # 🎯 BUSINESS LOGIC (Domain Layer)
│   │   ├── command/           # Write operations (Create, Update, Delete)
│   │   │   ├── create_rule.go
│   │   │   ├── update_limit.go
│   │   │   └── execute_validation.go
│   │   │
│   │   └── query/             # Read operations (List, Get, Search)
│   │       ├── list_rules.go
│   │       └── get_validation.go
│   │
│   └── adapters/              # 🔌 INFRASTRUCTURE (Adapters Layer)
│       ├── http/in/           # REST API (Fiber)
│       │   ├── routes.go      # Route definitions & middleware setup
│       │   ├── *_handler.go   # HTTP handlers (validation, rule, limit, audit)
│       │   └── middleware/    # Auth (API Key), IP extraction, CORS
│       │
│       ├── postgres/          # Database repositories
│       │   ├── rule_repo.go
│       │   ├── limit_repo.go
│       │   └── validation_repo.go
│       │
│       └── cel/               # CEL expression engine adapter
│           └── evaluator.go
│
├── pkg/
│   ├── model/                 # 📦 DOMAIN MODELS
│   │   ├── rule.go           # Fraud detection rules
│   │   ├── limit.go          # Spending limits
│   │   ├── validation.go     # Validation requests/responses
│   │   ├── transaction_validation.go  # Audit records
│   │   ├── context.go        # Account, Merchant, Segment contexts
│   │   └── transaction.go    # Transaction types & enums
│   │
│   └── constant/             # Shared constants
│       └── pagination.go     # Pagination defaults
│
├── migrations/               # Database migrations
├── docker-compose.yml        # Local development stack
├── Makefile                  # Development commands
└── .env.example              # Environment template
```

### Tech Stack

| Layer                | Technology                  | Purpose                                   |
|----------------------|-----------------------------|-------------------------------------------|
| **Language**         | Go 1.26.4                   | Performance, concurrency, static typing   |
| **HTTP Framework**   | Fiber v2.52.13              | Fast, Express-like API framework          |
| **Database**         | PostgreSQL 17               | ACID transactions, JSON support           |
| **Expression Engine**| CEL (google/cel-go v0.28.1) | Type-safe rule evaluation                 |
| **Observability**    | OpenTelemetry + Jaeger      | Distributed tracing                       |
| **Logging**          | Loki                        | Centralized log aggregation               |
| **Metrics**          | Prometheus                  | Time-series metrics                       |
| **Validation**       | validator/v10               | Struct tag validation                     |
| **Testing**          | Go testing + testify + Godog| Unit, integration & E2E BDD tests         |

---

## Quick Start

### Prerequisites

- Docker 20+ & Docker Compose 2+
- Go 1.26.4+ (for local development)
- Make (optional, for convenience commands)

### 1. Clone & Setup

```bash
# Tracer is co-located in the midaz monorepo
git clone https://github.com/LerianStudio/midaz.git
cd midaz/components/tracer

# Copy environment template
cp .env.example .env

# (Optional) Customize .env with your settings
vi .env
```

### 2. Start Services

```bash
# Start the Tracer app service (joins the shared infra-network;
# PostgreSQL and the OTel collector come from components/infra)
make up

# Verify the container is running
docker compose ps
```

### 3. Verify Health

```bash
# Check if app is running (liveness probe)
curl http://localhost:4020/health
# Expected: healthy

# Check if dependencies are ready (PostgreSQL)
curl http://localhost:4020/readyz
# Expected: {"status":"healthy","checks":{"postgres":{"status":"up","latency_ms":3,"tls":true}, "rule_cache":{"status":"up","latency_ms":1}}}
```

### Access Points

| Service        | URL                         | Credentials                                  |
|----------------|-----------------------------|--------------------------------------------- |
| **Tracer API** | <http://localhost:4020>     | API Key in `.env`                                  |
| **PostgreSQL** | `midaz-postgres-primary:5701` (shared infra) | user: `midaz`, pass: `lerian`, db: `tracer` |

### 4. Test API (Example)

```bash
# Set API key from .env
export API_KEY="your-api-key-from-env-file"

# Create a rule
curl -X POST http://localhost:4020/v1/rules \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "High-value transaction review",
    "description": "Flag transactions above $10,000 for manual review",
    "expression": "amount > 10000.00",
    "action": "REVIEW",
    "scopes": []
  }'
```

**Note:** Rules are created in `DRAFT` status and must be activated via `POST /v1/rules/{id}/activate` before they can be evaluated.

```bash
# Execute validation (minimal request)
curl -X POST http://localhost:4020/v1/validations \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "requestId": "123e4567-e89b-12d3-a456-426614174000",
    "transactionType": "CARD",
    "amount": "15000.00",
    "currency": "USD",
    "transactionTimestamp": "2026-01-28T10:30:00Z",
    "account": {
      "accountId": "223e4567-e89b-12d3-a456-426614174001"
    }
  }'
```

**Note:** `amount` is a decimal string value. Example: $15,000.00 = "15000.00".

---

## Development

### Make Commands

```bash
# Development
make run                # Start Tracer locally (outside Docker)
make build              # Compile binary to ./bin/tracer
make clean              # Remove build artifacts

# Testing
make test               # Run all tests
make test-unit          # Run unit tests only
make test-integration   # Run integration tests (with testcontainers)
make test-e2e           # Run E2E BDD tests (resets DB, runs Godog scenarios)
make test-all           # Run all tests (unit + integration)
make test-bench         # Run benchmark tests

# Coverage
make coverage-unit      # Unit test coverage (uses .ignorecoverunit)
make coverage-integration # Integration test coverage
make coverage           # All coverage targets

# Code Quality
make lint               # Run golangci-lint (requires install)
make format             # Format code with gofmt

# Docker
make up                 # Start all services
make down               # Stop all services
make restart            # Restart services
make rebuild-up         # Rebuild images and start

# Database
make migrate            # Apply migrations (when available)
make migrate-down       # Rollback last migration

# Documentation
# API docs are generated from the repo root: make generate-docs (covers ledger, tracer, reporter)

# Help
make help               # Show all available commands
```

### Development Workflow

```bash
# 1. Create feature branch
git checkout -b feature/my-feature

# 2. Write test first (TDD)
go test -v ./internal/services/command/... -run TestMyFeature

# 3. Implement feature
vi internal/services/command/my_feature.go

# 4. Run tests
make test

# 5. Run integration tests
make test-integration

# 6. Check code quality
make lint
make format

# 7. Commit (conventional commits)
git add .
git commit -m "feat: add high-value transaction alerts"

# 8. Push and create PR
git push origin feature/my-feature
```

### Code Conventions

- **Architecture:** Hexagonal + CQRS (see the root [PROJECT_RULES.md](../../docs/PROJECT_RULES.md) and tracer-specific [INVARIANTS.md](../../docs/tracer/INVARIANTS.md))
- **Testing:** TDD mandatory - write test before implementation
- **Coverage:** >=85% for all packages
- **Commits:** Conventional Commits format (`feat:`, `fix:`, `docs:`, etc.)
- **Linting:** golangci-lint (config: `.golangci.yml`)

---

## API Reference

### Base URL

```text
http://localhost:4020/v1
```

### Authentication

All endpoints require API Key header:

```http
X-API-Key: your-api-key
```

### Date/Time Format

**Query parameters** must use **RFC3339 with timezone**:

✅ Valid:

```text
2026-01-28T10:30:00Z          # UTC
2026-01-28T10:30:00-03:00     # São Paulo
```

❌ Invalid:

```text
2026-01-28                     # Missing time/timezone
2026-01-28T10:30:00            # Missing timezone
```

### Core Endpoints

| Method   | Endpoint                        | Description                                      |
|----------|---------------------------------|--------------------------------------------------|
| `POST`   | `/v1/validations`               | Execute transaction validation                   |
| `GET`    | `/v1/validations`               | List validation history (with filters)           |
| `GET`    | `/v1/validations/{id}`          | Get validation by ID                             |
| `GET`    | `/v1/audit-events`              | List audit events (with filters)                 |
| `GET`    | `/v1/audit-events/{id}`         | Get audit event by ID                            |
| `GET`    | `/v1/audit-events/{id}/verify`  | Verify hash chain integrity (SOX compliance)     |
| `POST`   | `/v1/rules`                     | Create fraud rule                                |
| `GET`    | `/v1/rules`                     | List rules                                       |
| `PATCH`  | `/v1/rules/{id}`                | Update rule                                      |
| `DELETE` | `/v1/rules/{id}`                | Delete rule                                      |
| `POST`   | `/v1/rules/{id}/activate`       | Activate rule                                    |
| `POST`   | `/v1/rules/{id}/deactivate`     | Deactivate rule                                  |
| `POST`   | `/v1/limits`                    | Create spending limit                            |
| `GET`    | `/v1/limits`                    | List limits                                      |
| `GET`    | `/v1/limits/{id}/usage`         | Get limit usage                                  |
| `PATCH`  | `/v1/limits/{id}`               | Update limit                                     |
| `POST`   | `/v1/limits/{id}/activate`      | Activate limit                                   |
| `DELETE` | `/v1/limits/{id}`               | Delete limit (DRAFT/INACTIVE only)               |
| `POST`   | `/v1/limits/{id}/deactivate`    | Deactivate limit                                 |

### Example: Execute Validation

**Minimal Request:**

```bash
POST /v1/validations
Content-Type: application/json
X-API-Key: your-api-key

{
  "requestId": "123e4567-e89b-12d3-a456-426614174000",
  "transactionType": "CARD",
  "amount": "5000.00",
  "currency": "USD",
  "transactionTimestamp": "2026-01-28T10:30:00Z",
  "account": {
    "accountId": "223e4567-e89b-12d3-a456-426614174001"
  }
}
```

**Complete Request (all optional fields):**

```bash
POST /v1/validations
Content-Type: application/json
X-API-Key: your-api-key

{
  "requestId": "123e4567-e89b-12d3-a456-426614174000",
  "transactionType": "CARD",
  "subType": "debit",
  "amount": "5000.00",
  "currency": "USD",
  "transactionTimestamp": "2026-01-28T10:30:00Z",
  "account": {
    "accountId": "223e4567-e89b-12d3-a456-426614174001",
    "type": "checking",
    "status": "active",
    "metadata": {
      "customer_tier": "gold"
    }
  },
  "segment": {
    "segmentId": "323e4567-e89b-12d3-a456-426614174002",
    "name": "VIP Customers"
  },
  "portfolio": {
    "portfolioId": "423e4567-e89b-12d3-a456-426614174003",
    "name": "Premium Portfolio"
  },
  "merchant": {
    "merchantId": "523e4567-e89b-12d3-a456-426614174004",
    "name": "Electronics Store",
    "category": "5732",
    "country": "US"
  },
  "metadata": {
    "device_id": "dev-12345",
    "ip_address": "192.168.1.1"
  }
}
```

**Notes:**
- `amount` is a decimal string value. Example: $5,000.00 = "5000.00"
- `transactionType` must be one of: `CARD`, `WIRE`, `PIX`, `CRYPTO`
- `account.type` values: `checking`, `savings`, `credit`
- `account.status` values: `active`, `suspended`, `closed`
- `merchant.category` is 4-digit MCC code (ISO 18245)
- `merchant.country` is 2-letter ISO 3166-1 alpha-2 code

**Response:**

```json
{
  "validationId": "623e4567-e89b-12d3-a456-426614174005",
  "requestId": "123e4567-e89b-12d3-a456-426614174000",
  "decision": "ALLOW",
  "reason": "Transaction approved",
  "matchedRuleIds": [],
  "evaluatedRuleIds": ["723e4567-e89b-12d3-a456-426614174006"],
  "limitUsageDetails": [
    {
      "limitId": "823e4567-e89b-12d3-a456-426614174007",
      "limitAmount": "100000.00",
      "scope": "account:223e4567-e89b-12d3-a456-426614174001",
      "period": "DAILY",
      "currentUsage": "5000.00",
      "attemptedAmount": "5000.00",
      "exceeded": false
    }
  ],
  "processingTimeMs": 45
}
```

**Decision Values:**
- `ALLOW` - Transaction approved, all checks passed
- `DENY` - Transaction rejected (rule violation or limit exceeded)
- `REVIEW` - Suspicious activity detected, manual review required

---

## Testing

### Run Tests

```bash
# All tests with race detection
make test

# Unit tests only
make test-unit

# Integration tests
make test-integration

# End-to-end BDD tests (requires running Tracer instance)
make test-e2e                                    # Full run (resets DB)
make test-e2e E2E_SKIP_RESET=1                   # Reuse current DB
make test-e2e E2E_SERVER=http://myhost:9090      # Custom server

# All tests
make test-all

# Specific package
go test -v ./internal/services/command/...

# Unit test coverage with filtering
make coverage-unit
open ./reports/unit_coverage.out

# Parallel execution (faster)
go test -race -count=1 -p 4 ./...
```

### Test Structure

```text
internal/services/command/
├── create_rule.go
└── create_rule_test.go        # Test file (same package)

tests/integration/             # Integration tests (testcontainers)
tests/end2end/                 # E2E BDD tests (Godog)
├── e2e_test.go                # Godog test runner
├── features/                  # Gherkin .feature files
│   ├── 01_rule_lifecycle.feature
│   ├── 02_limit_enforcement.feature
│   └── ...
├── steps/                     # Step definitions (Go)
│   ├── init.go                # Step registration
│   ├── auth_steps.go
│   ├── rule_steps.go
│   ├── validation_steps.go
│   ├── limit_steps.go
│   └── audit_steps.go
└── support/                   # Shared helpers
    ├── client.go              # HTTP client for Tracer API
    └── context.go             # Scenario context
```

### Test Patterns

**Unit Test Example:**

```go
func TestCreateRule_Success(t *testing.T) {
    // Arrange
    mockRepo := &MockRuleRepository{}
    service := NewCreateRuleService(mockRepo)

    // Act
    result, err := service.Execute(ctx, input)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

**Coverage Target:** >=85% for all packages

---

## Deployment

### Kubernetes (Recommended)

**Requirements:**
- Kubernetes 1.30+
- Helm 3.16+
- PostgreSQL 17 (managed or self-hosted)

**Helm Chart:** (TBD - planned for future release)

### Cloud Providers

- **AWS:** EKS + RDS PostgreSQL
- **GCP:** GKE + Cloud SQL
- **Azure:** AKS + Azure Database for PostgreSQL

### Environment Variables

See `.env.example` for all configuration options.

**Critical Variables:**

| Variable           | Description             | Default                                |
|--------------------|-------------------------|----------------------------------------|
| `SERVER_PORT`      | HTTP server port        | `4020`                                 |
| `DB_HOST`          | PostgreSQL host         | `midaz-postgres-primary`               |
| `DB_NAME`          | Database name           | `tracer`                               |
| `API_KEY`          | API authentication key  | (required)                             |
| `LOG_LEVEL`        | Logging verbosity       | `INFO`                                 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP trace exporter endpoint | `tracer-jaeger:4317`        |

---

## Documentation

| Document                                                      | Purpose                             | Audience          |
|---------------------------------------------------------------|-------------------------------------|-------------------|
| [PROJECT_RULES.md](../../docs/PROJECT_RULES.md)               | Architecture patterns & conventions (monorepo-wide) | Developers |
| [tracer/INVARIANTS.md](../../docs/tracer/INVARIANTS.md)       | Tracer-specific invariants (CEL, hash-chained audit, migration renumbering, latency budget) | Developers |

---

## Contributing

Contributions are welcome! Please follow these guidelines:

### Workflow

1. **Fork** the repository
2. **Create** a feature branch: `git checkout -b feature/my-feature`
3. **Write tests** first (TDD approach)
4. **Implement** the feature/fix
5. **Run** tests: `make test`
6. **Run** integration tests: `make test-integration`
7. **Run** E2E tests: `make test-e2e`
8. **Lint** code: `make lint`
9. **Commit** with conventional format: `feat: add webhook notifications`
10. **Push** and create a Pull Request

### Standards

- **Tests required** - No PR without tests (>=85% coverage)
- **Linting** - Code must pass golangci-lint
- **Documentation** - Update README/docs for user-facing changes
- **Commits** - Follow [Conventional Commits](https://www.conventionalcommits.org/)

---

## Support

- **Issues:** [GitHub Issues](https://github.com/LerianStudio/midaz/issues)
- **Discussions:** [GitHub Discussions](https://github.com/LerianStudio/midaz/discussions)
- **Contact:** LerianStudio Engineering Team

---

## License

This project is licensed under the Elastic License 2.0.
You are free to use, modify, and distribute this software, but you may not provide it to third parties as a hosted or managed service.
See the [LICENSE](LICENSE) file for details.

---

Built with ❤️ by LerianStudio Engineering Team
