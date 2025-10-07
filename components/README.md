# Midaz Components

## Overview

The `components` directory contains the main services and applications that make up the Midaz ledger platform. Each component is independently deployable and follows hexagonal architecture principles.

## Components

### 1. Onboarding Service

**Purpose:** Entity lifecycle management (organizations, ledgers, accounts, assets)

**Responsibilities:**

- Create, read, update, delete core entities
- Enforce business rules and validation
- Manage entity metadata
- Coordinate with transaction service via RabbitMQ
- Support hierarchical structures

**Technology Stack:**

- Go + Fiber (HTTP framework)
- PostgreSQL (entity storage)
- MongoDB (metadata storage)
- RabbitMQ (async messaging)
- Redis (caching)

**Documentation:** See [onboarding/README.md](onboarding/README.md)

---

### 2. Transaction Service

**Purpose:** Financial transaction processing and balance management

**Responsibilities:**

- Process financial transactions
- Manage account balances
- Record operations (debits, credits, holds, releases)
- Enforce double-entry accounting
- Support transaction routing
- Publish events and audit logs
- Async processing for high throughput

**Technology Stack:**

- Go + Fiber (HTTP framework)
- PostgreSQL (transaction/balance storage)
- MongoDB (metadata storage)
- RabbitMQ (async processing + events)
- Redis (caching + idempotency + locking)

**Documentation:** See [transaction/README.md](transaction/README.md)

---

### 3. MDZ CLI

**Purpose:** Command-line interface for Midaz platform

**Responsibilities:**

- Interact with Midaz APIs
- Manage entities via terminal
- Authenticate users
- Configure API endpoints
- Provide developer-friendly interface

**Technology Stack:**

- Go + Cobra (CLI framework)
- HTTP client for API calls
- OAuth/JWT authentication

**Documentation:** See [mdz/README.md](mdz/README.md)

---

### 4. Console (Frontend)

**Purpose:** Web-based user interface for Midaz platform

**Responsibilities:**

- Visual entity management
- Transaction monitoring
- Dashboard and analytics
- User authentication
- Admin operations

**Technology Stack:**

- Next.js + React + TypeScript
- Tailwind CSS
- Storybook (component library)
- Playwright (E2E testing)
- Jest (unit testing)

**Documentation:** See [console/README.md](console/README.md)

---

### 5. Infrastructure

**Purpose:** Development and deployment infrastructure

**Responsibilities:**

- Docker Compose configurations
- Database initialization scripts
- Monitoring setup (Grafana)
- Local development environment

**Technology Stack:**

- Docker + Docker Compose
- PostgreSQL + MongoDB
- RabbitMQ + Redis
- Grafana

**Documentation:** See [infra/README.md](infra/README.md)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         Console                              │
│                    (Web Frontend - Next.js)                  │
└────────────────────────┬────────────────────────────────────┘
                         │ HTTP/REST
                         ↓
┌─────────────────────────────────────────────────────────────┐
│                      API Gateway                             │
│                  (Authentication/Routing)                    │
└───────────┬──────────────────────────┬──────────────────────┘
            │                          │
            ↓                          ↓
┌─────────────────────┐    ┌─────────────────────────────────┐
│ Onboarding Service  │    │    Transaction Service          │
│                     │    │                                  │
│ - Organizations     │    │ - Transactions                   │
│ - Ledgers          │    │ - Balances                       │
│ - Accounts         │◄───┤ - Operations                     │
│ - Assets           │    │ - Routing                        │
│ - Portfolios       │    │ - Events                         │
└──────┬──────────────┘    └─────────┬───────────────────────┘
       │                              │
       │         RabbitMQ             │
       └──────────────┬───────────────┘
                      │
       ┌──────────────┴───────────────┬──────────────────┐
       ↓                              ↓                  ↓
┌─────────────┐              ┌──────────────┐    ┌────────────┐
│ PostgreSQL  │              │   MongoDB    │    │   Redis    │
│ (Entities)  │              │  (Metadata)  │    │  (Cache)   │
└─────────────┘              └──────────────┘    └────────────┘
```

## Service Communication

### Synchronous (HTTP/REST)

- Console → Onboarding Service
- Console → Transaction Service
- MDZ CLI → Onboarding Service
- MDZ CLI → Transaction Service

### Asynchronous (RabbitMQ)

- Onboarding → Transaction (account creation events)
- Transaction → Transaction (async transaction processing)
- Transaction → External (transaction events, audit logs)

### Data Stores

- **PostgreSQL**: Primary entity storage (ACID compliance)
- **MongoDB**: Flexible metadata storage (schema-less)
- **Redis**: Caching, idempotency, balance locking

## Development

### Running Services Locally

```bash
# Start infrastructure (PostgreSQL, MongoDB, RabbitMQ, Redis)
(cd components/infra && make up)

# Start onboarding service
(cd components/onboarding && make run)

# Start transaction service
(cd components/transaction && make run)

# Start console
(cd components/console && npm install && npm run dev)
```

### Running Tests

```bash
# Run all tests
make test

# Run integration tests
make test-integration

# Run E2E tests
cd tests/e2e
make test
```

## Documentation

Each component has comprehensive documentation:

- **README.md**: Component overview and architecture
- **internal/README.md**: Internal architecture and layers
- **internal/services/README.md**: Business logic documentation
- **internal/adapters/README.md**: Infrastructure layer documentation
- **internal/bootstrap/README.md**: Application initialization

## Related Documentation

- [Main README](../README.md) - Project overview
- [CONTRIBUTING.md](../CONTRIBUTING.md) - Contribution guidelines
- [STRUCTURE.md](../STRUCTURE.md) - Project structure
- [BUGS.md](../BUGS.md) - Known issues and bugs
