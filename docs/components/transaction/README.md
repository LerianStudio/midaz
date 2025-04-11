# Transaction Service

**Navigation:** [Home](../../) > [Components](../) > Transaction Service

## Overview

The Transaction Service is a core component of the Midaz system responsible for processing financial transactions and managing account balances. It implements a double-entry accounting model to ensure financial integrity and provides a RESTful API for transaction management.

## Responsibilities

- **Transaction Processing**: Create and manage financial transactions
- **Balance Management**: Track and update account balances
- **Operation Handling**: Record individual debit and credit operations
- **Asset Rate Management**: Track exchange rates between different assets
- **Double-Entry Enforcement**: Ensure debits equal credits in all transactions
- **Audit Trail**: Maintain complete history of financial activities

## Architecture

The Transaction Service follows a hexagonal architecture (ports and adapters) with clear separation between domain logic and external integrations:

```
┌─────────────────────────────────────────────────────────┐
│                  Transaction Service                     │
│                                                         │
│  ┌───────────────┐     ┌───────────────────────────┐    │
│  │   HTTP API    │     │       Domain Model        │    │
│  │  Controllers  │     │  (Entities & Validation)  │    │
│  └───────┬───────┘     └───────────┬───────────────┘    │
│          │                         │                    │
│          ▼                         │                    │
│  ┌───────────────┐                 │                    │
│  │   Services    │◄────────────────┘                    │
│  │               │                                      │
│  │  ┌─────────┐  │     ┌───────────────────────────┐    │
│  │  │Commands │  │     │         Adapters          │    │
│  │  └─────────┘  │     │                           │    │
│  │               │     │  ┌─────────┐ ┌─────────┐  │    │
│  │  ┌─────────┐  │     │  │Postgres│ │MongoDB │  │    │
│  │  │ Queries │  │─────┼─►│Adapter  │ │Adapter │  │    │
│  │  └─────────┘  │     │  └─────────┘ └─────────┘  │    │
│  └───────────────┘     │                           │    │
│                        │  ┌─────────┐ ┌─────────┐  │    │
│                        │  │RabbitMQ│ │ Redis   │  │    │
│                        │  │Adapter  │ │Adapter │  │    │
│                        │  └─────────┘ └─────────┘  │    │
│                        └───────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

The service implements the Command Query Responsibility Segregation (CQRS) pattern:

- **Commands**: Handle write operations (create transaction, update balance)
- **Queries**: Handle read operations (get transaction, list balances)

## Key Features

### Double-Entry Accounting

The service enforces double-entry accounting principles:

- Every transaction must have balanced debits and credits
- Source accounts (debits) and destination accounts (credits)
- Sum of debits must equal sum of credits
- Validation prior to processing

### Transaction Processing

Multiple methods for creating transactions:

- **JSON API**: Direct REST API calls
- **Template-based**: Reusing transaction templates
- **Asynchronous Processing**: Event-driven updates via message queues
- **Transaction Validation**: Comprehensive validation before processing

### Balance Management

Sophisticated balance tracking for accounts:

- **Available Balance**: Funds currently available for use
- **On-Hold Balance**: Funds reserved but not yet finalized
- **Asset-Specific Tracking**: Separate balances for each asset type
- **Optimistic Concurrency**: Version-based controls to prevent race conditions

### Asset Rate Management

Support for multi-currency operations:

- **Exchange Rate Tracking**: Store and update exchange rates
- **Rate-based Conversions**: Convert between different assets
- **Historical Rates**: Store rate history for auditing

### Event-Driven Architecture

The service uses event-driven patterns for scalability:

- **RabbitMQ Integration**: Publish and consume events for transaction processing
- **Asynchronous Processing**: Background processing of long-running operations
- **Idempotent Operations**: Safe retry mechanisms

## RESTful API

The service exposes a comprehensive RESTful API:

- `/v1/organizations/:organization_id/ledgers/:ledger_id/transactions` - Transaction management
- `/v1/organizations/:organization_id/ledgers/:ledger_id/operations` - Operation management
- `/v1/organizations/:organization_id/ledgers/:ledger_id/balances` - Balance queries
- `/v1/organizations/:organization_id/ledgers/:ledger_id/asset-rates` - Asset rate management

The API supports standard HTTP methods (GET, POST, PATCH) with consistent request/response formats and error handling.

## Data Persistence

The service uses multiple storage backends:

- **PostgreSQL**: Primary storage for transactions, operations, and balances
- **MongoDB**: Storage for flexible metadata
- **RabbitMQ**: Message queues for event processing
- **Redis**: Caching and temporary storage

## Integration Points

The Transaction Service integrates with other Midaz components:

- **Onboarding Service**: References entities like organizations, ledgers, and accounts
- **MDZ CLI**: Exposes API for command-line transaction management
- **Infrastructure Services**: Utilizes databases and message queues

## API Documentation

Comprehensive API documentation is available in OpenAPI/Swagger format:

- [API Documentation](./api.md)

## Next Steps

- [API Documentation](./api.md)
- [Domain Model](./domain-model.md)
- [Transaction Processing](./transaction-processing.md)
- [Balance Management](./balance-management.md)