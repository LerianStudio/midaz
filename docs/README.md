# Midaz System Documentation

This documentation provides a comprehensive overview of the Midaz system architecture, components, and how they interact. Use this as a reference when developing new features or understanding the existing codebase.

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture](#architecture)
3. [Components](#components)
   - [Infra](#infra)
   - [MDZ (CLI)](#mdz-cli)
   - [Onboarding](#onboarding)
   - [Transaction](#transaction)
4. [Shared Packages](#shared-packages)
5. [Data Models](#data-models)
6. [API Reference](#api-reference)
7. [Development Workflow](#development-workflow)

## System Overview

Midaz is an open-source ledger system designed to provide a comprehensive, multi-asset, multi-currency, and immutable ledger solution for modern financial applications. It serves as the foundation of a Core Banking Platform being developed by Lerian Studio.

The system follows a microservices architecture with separate components for different aspects of the system:

- **Infra**: Infrastructure services for the system (databases, message queues, etc.)
- **MDZ**: Command-line interface for interacting with the Midaz system
- **Onboarding**: Core service for managing organizations, ledgers, accounts, assets, portfolios, and segments
- **Transaction**: Service for handling financial transactions, operations, and balances

## Architecture

Midaz follows a microservices architecture with a clear separation of concerns. The system is built using Go and follows the Command Query Responsibility Segregation (CQRS) pattern for better scalability and maintainability.

### Key Architectural Principles

1. **Microservices**: Each component is a separate service with its own API and database.
2. **CQRS Pattern**: Separation of command (write) and query (read) operations.
3. **Double-Entry Accounting**: All financial transactions follow double-entry accounting principles.
4. **API-First Design**: All services expose RESTful APIs for integration.
5. **Cloud-Native**: Designed to run in containerized environments.

### System Interaction Flow

1. Clients interact with the system through:
   - RESTful APIs (for applications)
   - Command-line interface (for developers and administrators)

2. The Onboarding service manages the core entities (organizations, ledgers, accounts, etc.)

3. The Transaction service handles all financial transactions and maintains balances

4. Both services use shared data models defined in the `pkg` directory

## Components

### Infra

The infrastructure component provides all the necessary services for running the Midaz system:

- **PostgreSQL**: Primary database for storing transaction data with primary-replica setup
- **MongoDB**: Document database for storing certain types of data
- **Redis**: In-memory data store for caching and session management
- **RabbitMQ**: Message queue for asynchronous communication between services
- **OpenTelemetry**: Monitoring and observability solution

#### Key Files:
- `docker-compose.yml`: Defines all infrastructure services
- `.env.example`: Example environment variables for configuration

### MDZ (CLI)

The MDZ component is a command-line interface for interacting with the Midaz system. It provides commands for managing all aspects of the system:

- **Organization Management**: Create, read, update, delete organizations
- **Ledger Management**: Create, read, update, delete ledgers
- **Account Management**: Create, read, update, delete accounts
- **Asset Management**: Create, read, update, delete assets
- **Portfolio Management**: Create, read, update, delete portfolios
- **Segment Management**: Create, read, update, delete segments

#### Command Structure:

```
mdz
├── version
├── login
├── organization
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── ledger
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── asset
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── portfolio
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── segment
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── account
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
└── configure
```

### Onboarding

The Onboarding component is the core service for managing the fundamental entities in the Midaz system:

- **Organizations**: Top-level entities representing companies or business units
- **Ledgers**: Financial ledgers within organizations
- **Accounts**: Financial accounts within ledgers
- **Assets**: Asset types (currencies, commodities, etc.)
- **Portfolios**: Collections of accounts for grouping and reporting
- **Segments**: Business segments for categorizing accounts

#### Architecture:

The Onboarding service follows a clean architecture pattern with clear separation of concerns:

- **API Layer**: RESTful API endpoints defined in OpenAPI specification
- **Adapters**: Interface adapters for external systems (HTTP, databases, message queues)
- **Services**: Business logic divided into command (write) and query (read) services
- **Domain Models**: Core business entities and rules

#### Key Directories:
- `api/`: API definitions and documentation
- `cmd/app/`: Application entry point
- `internal/adapters/`: Interface adapters
- `internal/bootstrap/`: Application bootstrapping
- `internal/services/`: Business logic services
- `migrations/`: Database migration scripts

### Transaction

The Transaction component handles all financial transactions in the Midaz system:

- **Transactions**: Financial transactions between accounts
- **Operations**: Individual debit/credit operations within transactions
- **Balances**: Account balances resulting from transactions
- **Asset Rates**: Exchange rates between different assets

#### Architecture:

Similar to the Onboarding service, the Transaction service follows a clean architecture pattern:

- **API Layer**: RESTful API endpoints defined in OpenAPI specification
- **Adapters**: Interface adapters for external systems
- **Services**: Business logic divided into command and query services
- **Domain Models**: Core business entities and rules

#### Key Directories:
- `api/`: API definitions and documentation
- `cmd/app/`: Application entry point
- `internal/adapters/`: Interface adapters
- `internal/bootstrap/`: Application bootstrapping
- `internal/services/`: Business logic services
- `migrations/`: Database migration scripts

## Shared Packages

The `pkg` directory contains shared packages used by multiple components:

- **constant**: Shared constants and error definitions
- **mmodel**: Shared data models
- **gold**: Gold transaction parser and utilities
- **net**: Network utilities
- **shell**: Shell utilities

### Key Models:

- **Organization**: Top-level entity representing a company or business unit
- **Ledger**: Financial ledger within an organization
- **Account**: Financial account within a ledger
- **Asset**: Asset type (currency, commodity, etc.)
- **Portfolio**: Collection of accounts for grouping and reporting
- **Segment**: Business segment for categorizing accounts
- **Transaction**: Financial transaction between accounts
- **Operation**: Individual debit/credit operation within a transaction
- **Balance**: Account balance resulting from transactions

## Data Models

### Core Models

#### Organization
```go
type Organization struct {
    ID                   string         // Unique identifier
    ParentOrganizationID *string        // Parent organization ID (optional)
    LegalName            string         // Legal name
    DoingBusinessAs      *string        // DBA name (optional)
    LegalDocument        string         // Legal document (e.g., tax ID)
    Address              Address        // Physical address
    Status               Status         // Status (active, inactive, etc.)
    CreatedAt            time.Time      // Creation timestamp
    UpdatedAt            time.Time      // Last update timestamp
    DeletedAt            *time.Time     // Deletion timestamp (optional)
    Metadata             map[string]any // Custom metadata
}
```

#### Ledger
```go
type Ledger struct {
    ID             string         // Unique identifier
    Name           string         // Ledger name
    OrganizationID string         // Parent organization ID
    Status         Status         // Status (active, inactive, etc.)
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}
```

#### Account
```go
type Account struct {
    ID              string         // Unique identifier
    Name            string         // Account name
    ParentAccountID *string        // Parent account ID (optional)
    EntityID        *string        // Associated entity ID (optional)
    AssetCode       string         // Asset code (e.g., USD, BTC)
    OrganizationID  string         // Parent organization ID
    LedgerID        string         // Parent ledger ID
    PortfolioID     *string        // Associated portfolio ID (optional)
    SegmentID       *string        // Associated segment ID (optional)
    Status          Status         // Status (active, inactive, etc.)
    Alias           *string        // Account alias (optional)
    Type            string         // Account type
    CreatedAt       time.Time      // Creation timestamp
    UpdatedAt       time.Time      // Last update timestamp
    DeletedAt       *time.Time     // Deletion timestamp (optional)
    Metadata        map[string]any // Custom metadata
}
```

#### Transaction
```go
type Transaction struct {
    ID                       string                     // Unique identifier
    ParentTransactionID      *string                    // Parent transaction ID (optional)
    Description              string                     // Transaction description
    Template                 string                     // Transaction template
    Status                   Status                     // Status
    Amount                   *int64                     // Transaction amount
    AmountScale              *int64                     // Amount scale (decimal places)
    AssetCode                string                     // Asset code (e.g., USD, BTC)
    ChartOfAccountsGroupName string                     // Chart of accounts group
    Source                   []string                   // Source accounts
    Destination              []string                   // Destination accounts
    LedgerID                 string                     // Parent ledger ID
    OrganizationID           string                     // Parent organization ID
    Body                     libTransaction.Transaction // Transaction body
    CreatedAt                time.Time                  // Creation timestamp
    UpdatedAt                time.Time                  // Last update timestamp
    DeletedAt                *time.Time                 // Deletion timestamp (optional)
    Metadata                 map[string]any             // Custom metadata
    Operations               []*operation.Operation     // Associated operations
}
```

## API Reference

### Onboarding API

The Onboarding API provides endpoints for managing organizations, ledgers, accounts, assets, portfolios, and segments.

#### Organizations
- `GET /v1/organizations`: Get all organizations
- `POST /v1/organizations`: Create an organization
- `GET /v1/organizations/{id}`: Get an organization by ID
- `PATCH /v1/organizations/{id}`: Update an organization
- `DELETE /v1/organizations/{id}`: Delete an organization

#### Ledgers
- `GET /v1/organizations/{organization_id}/ledgers`: Get all ledgers
- `POST /v1/organizations/{organization_id}/ledgers`: Create a ledger
- `GET /v1/organizations/{organization_id}/ledgers/{id}`: Get a ledger by ID
- `PATCH /v1/organizations/{organization_id}/ledgers/{id}`: Update a ledger
- `DELETE /v1/organizations/{organization_id}/ledgers/{id}`: Delete a ledger

#### Accounts
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`: Get all accounts
- `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`: Create an account
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}`: Get an account by ID
- `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}`: Update an account
- `DELETE /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}`: Delete an account

#### Assets
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets`: Get all assets
- `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets`: Create an asset
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}`: Get an asset by ID
- `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}`: Update an asset
- `DELETE /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}`: Delete an asset

#### Portfolios
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios`: Get all portfolios
- `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios`: Create a portfolio
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}`: Get a portfolio by ID
- `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}`: Update a portfolio
- `DELETE /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}`: Delete a portfolio

#### Segments
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments`: Get all segments
- `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments`: Create a segment
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}`: Get a segment by ID
- `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}`: Update a segment
- `DELETE /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}`: Delete a segment

### Transaction API

The Transaction API provides endpoints for managing transactions, operations, balances, and asset rates.

#### Balances
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/balances`: Get all balances
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances`: Get all balances by account ID
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id`: Get a balance by ID
- `DELETE /v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id`: Delete a balance

#### Operations
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations`: Get all operations by account ID

#### Asset Rates
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code}`: Get asset rates by asset code

#### Transactions
- `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl`: Create a transaction with DSL
- `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`: Get a transaction by ID
- `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert`: Revert a transaction

## Development Workflow

### Setting Up the Development Environment

1. Clone the repository:
   ```bash
   git clone https://github.com/LerianStudio/midaz.git
   cd midaz
   ```

2. Set up environment variables:
   ```bash
   make set-env
   ```

3. Start all services:
   ```bash
   make up
   ```

### Running Tests

```bash
make test
```

### Formatting Code

```bash
make format
```

### Running Linter

```bash
make lint
```

### Setting Up Git Hooks

```bash
make setup-git-hooks
```

### Building the CLI

```bash
cd components/mdz
make build
```

### Running Migrations

```bash
cd components/onboarding
make migrate-up
```

```bash
cd components/transaction
make migrate-up
```
