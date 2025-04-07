# Onboarding Service

**Navigation:** [Home](../../) > [Components](../) > Onboarding Service

## Overview

The Onboarding Service is a core component of the Midaz system responsible for managing the financial entity hierarchy. It implements a RESTful API that enables the creation and management of organizations, ledgers, assets, and accounts that form the foundation of the financial system.

## Responsibilities

- **Entity Management**: Create, read, update, and delete operations for all financial entities
- **Hierarchy Enforcement**: Maintain proper parent-child relationships between entities
- **Metadata Management**: Store and retrieve flexible metadata for entities
- **Data Validation**: Enforce business rules and data integrity constraints
- **API Provision**: Expose RESTful endpoints for entity management

## Architecture

The Onboarding Service follows a hexagonal architecture (also known as ports and adapters) with clear separation between:

- **Domain Logic**: Core business rules in the service layer
- **Adapters**: Integration with external systems like databases and message queues
- **API**: HTTP endpoints exposing functionality

Additionally, it implements the Command Query Responsibility Segregation (CQRS) pattern:

- **Commands**: Handle write operations (create, update, delete)
- **Queries**: Handle read operations (get, list)

```
┌─────────────────────────────────────────────────────────┐
│                  Onboarding Service                      │
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

## Key Features

### Entity Hierarchy Management

The service manages the complete financial entity hierarchy:

- **Organizations**: Top-level entities with organizational details
- **Ledgers**: Financial record-keeping systems within organizations
- **Assets**: Financial instruments like currencies and cryptocurrencies
- **Segments**: Logical divisions like business areas or product lines
- **Portfolios**: Collections of accounts for specific purposes
- **Accounts**: Basic units for tracking financial resources

### RESTful API

The service exposes a comprehensive RESTful API organized hierarchically:

- `/v1/organizations` - Organization management
- `/v1/organizations/:organization_id/ledgers` - Ledger management
- `/v1/organizations/:organization_id/ledgers/:ledger_id/assets` - Asset management
- `/v1/organizations/:organization_id/ledgers/:ledger_id/segments` - Segment management
- `/v1/organizations/:organization_id/ledgers/:ledger_id/portfolios` - Portfolio management
- `/v1/organizations/:organization_id/ledgers/:ledger_id/accounts` - Account management

### Flexible Metadata

The service supports flexible metadata for all entities:

- Key-value pairs stored in MongoDB
- Queryable via API
- No schema constraints for maximum flexibility

### Data Persistence

The service uses multiple storage backends:

- **PostgreSQL**: Primary storage for entity data with strong consistency
- **MongoDB**: Storage for flexible metadata
- **RabbitMQ**: Message publishing for event-driven architecture
- **Redis**: Caching and temporary data storage

## Integration Points

The Onboarding Service integrates with other Midaz components:

- **Transaction Service**: Provides entity data for transaction processing
- **MDZ CLI**: Exposes API for command-line management
- **Infrastructure Services**: Utilizes databases and message queues

## API Documentation

Comprehensive API documentation is available in OpenAPI/Swagger format:

- [API Documentation](./api.md)

## Next Steps

- [API Documentation](./api.md)
- [Domain Model](./domain-model.md)
- [Service Architecture](./service-architecture.md)