# Midaz System Architecture

This document provides a detailed overview of the Midaz system architecture, explaining how the different components interact and the design principles behind the system.

## Architectural Overview

Midaz follows a microservices architecture with clear separation of concerns. The system is built using Go and follows the Command Query Responsibility Segregation (CQRS) pattern for better scalability and maintainability.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Client Applications                      │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                            MDZ CLI                              │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                           API Gateway                           │
└───────────┬─────────────────────────────────────┬───────────────┘
            │                                     │
            ▼                                     ▼
┌───────────────────────────┐       ┌───────────────────────────┐
│     Onboarding Service    │       │    Transaction Service    │
│                           │       │                           │
│  ┌───────────────────┐    │       │  ┌───────────────────┐    │
│  │  Command Service  │    │       │  │  Command Service  │    │
│  └───────────────────┘    │       │  └───────────────────┘    │
│                           │       │                           │
│  ┌───────────────────┐    │       │  ┌───────────────────┐    │
│  │   Query Service   │    │       │  │   Query Service   │    │
│  └───────────────────┘    │       │  └───────────────────┘    │
└───────────┬───────────────┘       └───────────┬───────────────┘
            │                                   │
            ▼                                   ▼
┌───────────────────────────┐       ┌───────────────────────────┐
│      PostgreSQL DB        │       │      PostgreSQL DB        │
│     (Primary/Replica)     │       │     (Primary/Replica)     │
└───────────────────────────┘       └───────────────────────────┘
            │                                   │
            ▼                                   ▼
┌───────────────────────────┐       ┌───────────────────────────┐
│        MongoDB            │       │        MongoDB            │
└───────────────────────────┘       └───────────────────────────┘
            │                                   │
            └───────────────┬───────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                           RabbitMQ                              │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                            Redis                                │
└─────────────────────────────────────────────────────────────────┘
```

## Key Architectural Principles

### 1. Microservices Architecture

Midaz is built as a collection of loosely coupled services, each responsible for a specific domain:

- **Onboarding Service**: Manages organizations, ledgers, accounts, assets, portfolios, and segments
- **Transaction Service**: Handles financial transactions, operations, and balances

Each service:
- Has its own database
- Exposes a RESTful API
- Can be deployed and scaled independently
- Communicates with other services through well-defined interfaces

### 2. Command Query Responsibility Segregation (CQRS)

The CQRS pattern separates read and write operations into different models:

- **Command Services**: Handle write operations (create, update, delete)
- **Query Services**: Handle read operations (get, list)

Benefits of CQRS in Midaz:
- Improved performance by optimizing read and write operations separately
- Better scalability by allowing independent scaling of read and write services
- Enhanced security by applying different validation rules to commands and queries

### 3. Double-Entry Accounting

All financial transactions in Midaz follow double-entry accounting principles:

- Every transaction must have at least one debit and one credit
- The sum of all debits must equal the sum of all credits
- Transactions are immutable once committed

This ensures:
- Financial integrity
- Auditability
- Compliance with accounting standards

### 4. API-First Design

Midaz follows an API-first approach:

- All services expose RESTful APIs defined using OpenAPI specification
- APIs are versioned to ensure backward compatibility
- Authentication and authorization are handled at the API level
- Comprehensive API documentation is provided

### 5. Cloud-Native Design

Midaz is designed to run in containerized environments:

- All components can be deployed as Docker containers
- Infrastructure is defined as code using Docker Compose
- Services are stateless and can be scaled horizontally
- Configuration is managed through environment variables

## Data Flow

### Organization and Ledger Setup

1. Client creates an organization through the Onboarding API
2. Client creates a ledger within the organization
3. Client sets up accounts, assets, portfolios, and segments within the ledger

### Transaction Processing

1. Client submits a transaction through the Transaction API
2. Transaction service validates the transaction (double-entry, sufficient funds, etc.)
3. Transaction service creates the transaction and associated operations
4. Transaction service updates account balances
5. Transaction service publishes events to RabbitMQ for downstream processing

### Query Processing

1. Client requests data through the Query API
2. Query service retrieves data from the database
3. Query service formats and returns the data to the client

## Infrastructure Components

### PostgreSQL

PostgreSQL is used as the primary database for storing structured data:

- Organizations, ledgers, accounts, assets, portfolios, segments
- Transactions, operations, balances
- Configured with primary-replica setup for high availability

### MongoDB

MongoDB is used for storing document-based data:

- Complex metadata
- Audit logs
- Configuration data

### RabbitMQ

RabbitMQ is used for asynchronous communication between services:

- Event-driven architecture
- Publish-subscribe pattern
- Message queuing for reliable delivery

### Redis

Redis is used for:

- Caching
- Session management
- Rate limiting
- Distributed locking

### OpenTelemetry

OpenTelemetry is used for monitoring and observability:

- Distributed tracing
- Metrics collection
- Logging
- Alerting

## Security Architecture

### Authentication and Authorization

- Token-based authentication using JWT
- Role-based access control (RBAC)
- Multi-tenancy with organization-level isolation
- API key management for service-to-service communication

### Data Protection

- Encryption at rest for sensitive data
- TLS for all network communication
- Secure handling of credentials and secrets
- Regular security audits and penetration testing

### Compliance

- SOC-2 ready
- GDPR compliant
- PCI-DSS ready
- Audit logging for all operations

## Deployment Architecture

### Development Environment

- Local Docker Compose setup
- Mock services for external dependencies
- Development tools and utilities

### Testing Environment

- Automated testing pipeline
- Integration tests with real dependencies
- Performance and load testing

### Production Environment

- Kubernetes orchestration
- Auto-scaling based on load
- High availability configuration
- Disaster recovery procedures

## Future Architecture Considerations

- Serverless functions for event processing
- GraphQL API for more flexible queries
- Event sourcing for complete audit trail
- Multi-region deployment for global availability
