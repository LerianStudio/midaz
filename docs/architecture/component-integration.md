# Component Integration

**Navigation:** [Home](../../) > [Architecture](../) > Component Integration

This document describes how the different components in Midaz integrate and communicate with each other to form a cohesive system.

## Overview

Midaz follows a microservices architecture where each component has a specific responsibility and communicates with other components through well-defined interfaces. This architecture enables:

- Independent development and deployment of components
- Scalability of individual components based on demand
- Resilience through isolation of failures
- Flexibility to use appropriate technologies for each component

The primary integration patterns used in Midaz are:

1. **REST APIs** for synchronous service-to-service communication
2. **Event-driven messaging** for asynchronous communication
3. **Shared infrastructure** for persistent storage and messaging

## Component Interaction Diagram

```
┌─────────────────┐          ┌───────────────────┐
│                 │  REST    │                   │
│    MDZ CLI      ├─────────►│  Onboarding       │
│                 │          │  Service          │
└─────────────────┘          └───────┬───────────┘
                                     │ 
                                     │ Events
                                     ▼
┌─────────────────┐          ┌───────────────────┐
│                 │  REST    │                   │
│  External Apps  ├─────────►│  Transaction      │
│                 │          │  Service          │
└─────────────────┘          └───────┬───────────┘
                                     │
                                     │
              ┌─────────────────────┬┴──────────────┐
              │                     │               │
              ▼                     ▼               ▼
┌─────────────────┐      ┌─────────────────┐  ┌───────────────┐
│                 │      │                 │  │               │
│   PostgreSQL    │      │    MongoDB      │  │   RabbitMQ    │
│                 │      │                 │  │               │
└─────────────────┘      └─────────────────┘  └───────────────┘
```

## Service-to-Service Communication

### Onboarding and Transaction Service Integration

The Onboarding and Transaction services interact via:

#### 1. Event-Based Communication

The primary integration mechanism between these services is event-based communication through RabbitMQ:

```
┌───────────────┐          ┌───────────┐         ┌────────────────┐
│  Onboarding   │          │           │         │  Transaction    │
│  Service      │          │  RabbitMQ │         │  Service       │
└───────┬───────┘          └─────┬─────┘         └────────┬───────┘
        │                        │                        │
        │ Create Account         │                        │
        │                        │                        │
        │ Publish Account Event  │                        │
        ├────────────────────────►                        │
        │                        │                        │
        │                        │ Consume Account Event  │
        │                        ├───────────────────────►│
        │                        │                        │
        │                        │                        │ Create Balance
        │                        │                        │ for Account
        │                        │                        │
```

Key event types include:
- Account creation events
- Account updates
- Account deletion (soft delete)

The event structure follows a standard format:

```json
{
  "organization_id": "uuid",
  "ledger_id": "uuid",
  "account_id": "uuid",
  "payload": {
    // Entity-specific data
  }
}
```

#### 2. Implicit Data Dependency

Transaction service has an implicit dependency on the Onboarding service's data model:
- References organization, ledger, and account IDs
- Expects these entities to exist before transactions can be created
- Uses the same identifier scheme for entity references

### CLI and Service Integration

The MDZ CLI interacts with both services through REST APIs:

1. **Authentication Flow**:
   - OAuth 2.0 authentication with token-based access
   - Token stored locally for subsequent requests
   - Supports browser-based and terminal-based login flows

2. **Resource Management**:
   - Creates and manages entities through REST API calls
   - Follows a hierarchical resource pattern
   - Uses standard HTTP methods (GET, POST, PATCH, DELETE)

3. **Error Handling**:
   - Consistent error response format across services
   - Detailed error messages for troubleshooting
   - Status codes follow HTTP conventions

## Network Configuration

Components are organized into logical networks to control communication:

- **infra-network**: Shared by all services and infrastructure components
- **onboarding-network**: Specific to the onboarding service
- **transaction-network**: Specific to the transaction service
- **plugin-auth-network**: Shared for authentication

This network segmentation provides:
- Isolation between components
- Control over service discovery
- Security through network boundaries

## API Contracts

### Onboarding Service API

The Onboarding Service exposes a REST API for entity management:

- `/v1/organizations` - Organization management
- `/v1/organizations/:org_id/ledgers` - Ledger management
- `/v1/organizations/:org_id/ledgers/:ledger_id/assets` - Asset management
- `/v1/organizations/:org_id/ledgers/:ledger_id/segments` - Segment management
- `/v1/organizations/:org_id/ledgers/:ledger_id/portfolios` - Portfolio management
- `/v1/organizations/:org_id/ledgers/:ledger_id/accounts` - Account management

### Transaction Service API

The Transaction Service exposes a REST API for financial operations:

- `/v1/organizations/:org_id/ledgers/:ledger_id/transactions` - Transaction management
- `/v1/organizations/:org_id/ledgers/:ledger_id/operations` - Operation management
- `/v1/organizations/:org_id/ledgers/:ledger_id/balances` - Balance management
- `/v1/organizations/:org_id/ledgers/:ledger_id/asset-rates` - Asset rate management

### API Documentation

Both services provide OpenAPI/Swagger documentation for their APIs, ensuring:
- Clear contract definition
- Consistent parameter and response formats
- Comprehensive error documentation

## Infrastructure Integration

### PostgreSQL Integration

Both services use PostgreSQL for persistent storage:

- Each service has its own database schema
- Transaction service depends on entities created by the Onboarding service
- Connection pooling for efficient database usage
- Migration scripts for schema versioning

### MongoDB Integration

MongoDB is used for metadata storage:

- Flexible schema for entity metadata
- Shared connection configuration
- Replica set for high availability

### RabbitMQ Integration

RabbitMQ provides the messaging infrastructure:

- Predefined exchanges and queues for known message types
- Direct exchange type for message routing
- Durable queues for message persistence
- Connection pooling and automatic reconnection

## Authentication and Authorization

Midaz implements secure communication between components:

1. **User Authentication**:
   - OAuth 2.0 flow for end-user authentication
   - JWT tokens for API access

2. **Service Authentication**:
   - Service-specific credentials for database access
   - Dedicated RabbitMQ users for each service

3. **API Authorization**:
   - Role-based access controls
   - Resource-level permissions
   - Organization and ledger context for all operations

## Data Flow Examples

### Account Creation and Balance Creation

```
┌───────────┐          ┌───────────────┐         ┌───────────┐        ┌────────────────┐
│           │          │               │         │           │        │                │
│   User    │          │   MDZ CLI     │         │ Onboarding│        │  Transaction   │
│           │          │               │         │  Service  │        │  Service       │
└─────┬─────┘          └───────┬───────┘         └─────┬─────┘        └────────┬───────┘
      │                        │                       │                       │
      │ Create Account Command │                       │                       │
      ├───────────────────────►│                       │                       │
      │                        │                       │                       │
      │                        │ POST /accounts        │                       │
      │                        ├──────────────────────►│                       │
      │                        │                       │                       │
      │                        │                       │ Store Account         │
      │                        │                       ├─────────┐             │
      │                        │                       │         │             │
      │                        │                       ◄─────────┘             │
      │                        │                       │                       │
      │                        │                       │ Publish Account Event │
      │                        │                       ├──────────────────────►│
      │                        │                       │                       │
      │                        │ 201 Created           │                       │ Create Balance
      │                        ◄──────────────────────┤                       ├─────────┐
      │                        │                       │                       │         │
      │ Account Created        │                       │                       │         │
      ◄───────────────────────┤                       │                       ◄─────────┘
      │                        │                       │                       │
```

### Transaction Creation

```
┌───────────┐          ┌───────────────┐                     ┌────────────────┐
│           │          │               │                     │                │
│   User    │          │   MDZ CLI     │                     │  Transaction   │
│           │          │               │                     │  Service       │
└─────┬─────┘          └───────┬───────┘                     └────────┬───────┘
      │                        │                                      │
      │ Create Transaction     │                                      │
      ├───────────────────────►│                                      │
      │                        │                                      │
      │                        │ POST /transactions                   │
      │                        ├─────────────────────────────────────►│
      │                        │                                      │
      │                        │                                      │ Validate Transaction
      │                        │                                      ├─────────┐
      │                        │                                      │         │
      │                        │                                      ◄─────────┘
      │                        │                                      │
      │                        │                                      │ Publish Transaction Event
      │                        │                                      ├─────────┐
      │                        │                                      │         │
      │                        │                                      ◄─────────┘
      │                        │                                      │
      │                        │ 202 Accepted                         │ Process Transaction Async
      │                        ◄─────────────────────────────────────┤ (Update Balances, Create
      │                        │                                      │  Operations, etc.)
      │ Transaction Created    │                                      │
      ◄───────────────────────┤                                      │
      │                        │                                      │
```

## Benefits of the Integration Approach

The integration approach used in Midaz provides several benefits:

1. **Loose Coupling**: Services can evolve independently
2. **Scalability**: Each component can scale based on its specific load
3. **Resilience**: Failures are isolated to specific components
4. **Flexibility**: Different technologies can be used for each component
5. **Clear Contracts**: Well-defined interfaces ensure proper integration

## Next Steps

- [Hexagonal Architecture](./hexagonal-architecture.md) - How components are structured internally
- [Event-Driven Design](./event-driven-design.md) - How events are used for integration
- [Entity Lifecycle](./data-flow/entity-lifecycle.md) - How entities flow through the system
- [Transaction Lifecycle](./data-flow/transaction-lifecycle.md) - How transactions are processed