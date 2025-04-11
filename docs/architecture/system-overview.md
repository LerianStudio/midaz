# System Overview

**Navigation:** [Home](../../) > [Architecture](../) > System Overview

This document provides a high-level overview of the Midaz system architecture, its major components, and their interactions.

## Introduction

Midaz is an enterprise-grade open-source ledger system designed for financial applications. It employs a modular microservices architecture following cloud-native design principles and implements hexagonal architecture patterns for clean separation of concerns.

## Architectural Diagram

For a detailed system architecture diagram, see the [System Architecture Diagram](../assets/system-architecture-diagram.md).

A simplified architecture overview:

```
+----------------------------------------------------------+
|                     Client Applications                   |
|    +------------------+      +----------------------+     |
|    |     MDZ CLI      |      |    External Apps     |     |
|    +------------------+      +----------------------+     |
+----------------------------------------------------------+
                |                       |
                |   REST APIs / Events  |
                v                       v
+----------------------------------------------------------+
|                    Service Layer                          |
|  +-------------------+         +--------------------+     |
|  | Onboarding Service|         | Transaction Service|     |
|  | (Entity Mgmt)     |<------->| (Financial Ops)    |     |
|  +-------------------+         +--------------------+     |
+----------------------------------------------------------+
                |                       |
                v                       v
+----------------------------------------------------------+
|                  Infrastructure Layer                     |
|  +----------+  +--------+  +---------+  +------------+   |
|  |PostgreSQL|  |MongoDB |  | RabbitMQ|  |Redis/Valkey|   |
|  +----------+  +--------+  +---------+  +------------+   |
|                                                          |
|  +----------+                        +----------------+  |
|  | Grafana  |                        | OpenTelemetry  |  |
|  +----------+                        +----------------+  |
+----------------------------------------------------------+
```

## Major Components

### Onboarding Service

The Onboarding Service is responsible for managing the core financial entities in the system:

- **Organizations**: Top-level entities with parent-child relationships
- **Ledgers**: Financial record-keeping systems belonging to organizations
- **Assets**: Different types of value with specific codes
- **Portfolios**: Collections of accounts
- **Segments**: Categories for grouping accounts
- **Accounts**: Basic units for tracking financial resources

Key features:
- Implementation of hexagonal architecture with CQRS pattern
- RESTful API with comprehensive OpenAPI documentation
- Storage of primary data in PostgreSQL and flexible metadata in MongoDB
- Full lifecycle management of all financial entities

### Transaction Service

The Transaction Service handles all financial transaction processing:

- **Transactions**: Financial transactions with debits and credits
- **Operations**: Individual entries in transactions
- **Balances**: Current financial position of accounts
- **Asset Rates**: Exchange rates between different assets

Key features:
- Complex n:n transaction support with double-entry accounting
- Multiple transaction creation methods (JSON, DSL, templates)
- Asset rate management and balance tracking
- Event-driven architecture using RabbitMQ for transaction lifecycle

### MDZ CLI

The MDZ CLI provides a command-line interface for interacting with the Midaz system:

- Built with Cobra for a modern CLI experience
- Interactive TUI components for improved usability
- Complete coverage of all service APIs
- Local configuration management and token-based authentication

### Infrastructure Layer

The infrastructure layer provides the foundational services for the Midaz system:

- **PostgreSQL**: Primary-replica setup for transactional/structured data
- **MongoDB**: Replica set for flexible metadata storage
- **RabbitMQ**: Message queuing with predefined exchanges for event-driven architecture
- **Redis/Valkey**: Caching and message passing
- **Grafana/OpenTelemetry**: Monitoring and observability

## Key Design Patterns

### Hexagonal Architecture

Midaz implements hexagonal architecture (also known as ports and adapters) to achieve clear separation between:

- **Domain Logic**: Core business rules and models
- **Application Services**: Use cases and application flow
- **Adapters**: Integration with external systems and user interfaces

This architecture allows the core business logic to remain isolated from external concerns like databases or UI, making the system more maintainable and testable.

### CQRS Pattern

The Command Query Responsibility Segregation pattern is implemented to separate:

- **Commands**: Operations that change the state of the system
- **Queries**: Operations that read data without modifying state

This separation allows for optimization of each path independently and simplifies complex domain logic.

### Event-Driven Architecture

Midaz utilizes event-driven architecture with RabbitMQ to enable:

- Asynchronous processing
- Loose coupling between services
- Scalability and resilience
- Complex transaction life cycle management

## Data Flow

### Entity Management Flow

1. Entity creation requests flow through the Onboarding Service API
2. Commands create and validate entities in PostgreSQL
3. Metadata is stored in MongoDB for flexibility
4. Events notify interested components of entity changes

### Transaction Processing Flow

1. Transaction requests come through the Transaction Service API
2. Transaction validation ensures correctness
3. Operations are created to record debits and credits
4. Balances are updated with optimistic concurrency control
5. Events signal completion or errors

## API Organization

The APIs in Midaz follow RESTful principles with:

- Resource-oriented design following the financial hierarchy
- OpenAPI/Swagger documentation
- Token-based authentication
- Standardized error responses
- Specialized endpoints for transaction processing

## Database Design Philosophy

Midaz employs a multi-database approach:

- **PostgreSQL**: Used for structured data requiring ACID compliance
- **MongoDB**: Used for flexible metadata storage
- **Redis**: Used for caching and temporary data

Database connections utilize connection pooling and separate read/write connections where appropriate.

## Next Steps

For more detailed information:

- [Hexagonal Architecture](./hexagonal-architecture.md) - Detailed explanation of the architectural pattern
- [Event-Driven Design](./event-driven-design.md) - How events are used in Midaz
- [Component Integration](./component-integration.md) - How components interact
- [Data Flow](./data-flow/) - Detailed data flow documentation