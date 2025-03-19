# Midaz Architecture Documentation

## Overview

Midaz is a sophisticated financial platform developed by Lerian Studio. It employs a microservice architecture with key components that handle different aspects of the financial system. This document provides an analysis of the core components from both technical and business perspectives.

## Table of Contents

- [Midaz Architecture Documentation](#midaz-architecture-documentation)
  - [Overview](#overview)
  - [Table of Contents](#table-of-contents)
  - [Onboarding Component](#onboarding-component)
    - [Onboarding Technical Analysis](#onboarding-technical-analysis)
    - [Onboarding Business Analysis](#onboarding-business-analysis)
    - [Onboarding Entity Relationships](#onboarding-entity-relationships)
  - [Transaction Component](#transaction-component)
    - [Transaction Technical Analysis](#transaction-technical-analysis)
    - [Transaction Business Analysis](#transaction-business-analysis)
    - [Transaction Entity Relationships](#transaction-entity-relationships)
  - [Infrastructure Component](#infrastructure-component)
    - [Infrastructure Technical Analysis](#infrastructure-technical-analysis)
    - [Infrastructure Services Overview](#infrastructure-services-overview)
    - [Infrastructure Integration Points](#infrastructure-integration-points)
  - [Integration Between Components](#integration-between-components)

## Onboarding Component

The onboarding component serves as the foundation for the Midaz platform, establishing the organizational and account structure that the rest of the system operates on.

### Onboarding Technical Analysis

1. **Architecture**:
   - Follows a microservice architecture (containerized with Docker)
   - Uses Go (Golang) as the programming language
   - Implements a hexagonal/ports and adapters architecture pattern with clear separation of:
     - Domain models (in pkg/mmodel)
     - Repositories (data access layers)
     - Services (business logic in command and query patterns)
     - API endpoints (HTTP adapters)

2. **Data Storage**:
   - Uses PostgreSQL for entity data (organizations, ledgers, accounts, etc.)
   - Uses MongoDB for flexible metadata storage
   - Implements repository patterns to abstract the data access

3. **Communication**:
   - Uses RabbitMQ for asynchronous messaging
   - Has Redis for caching/temporary storage
   - HTTP REST APIs for synchronous communication

4. **Observability**:
   - Implements OpenTelemetry for metrics, traces, and logging
   - Records business metrics for operations (creates, updates, etc.)
   - Tracks durations and errors

5. **Code Structure**:
   - Command pattern for write operations (create, update, delete)
   - Query pattern for read operations
   - Clean separation of concerns with dependency injection

### Onboarding Business Analysis

1. **Purpose**:
   - The onboarding component handles the initial setup and management of financial entities in the Midaz system
   - It's responsible for creating the hierarchical structure that other financial operations will use
   - Acts as the foundation for Lerian Studio's financial platform

2. **Business Workflow**:
   - Organizations are created first (top-level business entities)
   - Ledgers are then established within organizations 
   - Segments and Portfolios are created to organize financial activities
   - Accounts are created within the structure, associated with specific assets
   - This hierarchy supports complex financial operations and reporting

3. **Business Value**:
   - Provides enterprise-level organizational hierarchy
   - Supports multi-entity, multi-currency financial operations
   - Enables segmentation for different business units or purposes
   - Allows portfolio management for different entities (clients, users, etc.)
   - The meticulous entity structure suggests this is built for regulatory compliance and detailed financial management

4. **Flexibility and Extensibility**:
   - The metadata field on each entity allows for custom attributes without schema changes
   - Status tracking on all entities enables lifecycle management
   - Hierarchical relationships support complex organizational structures
   - The system appears designed for financial institutions or companies with complex financial needs

### Onboarding Entity Relationships

1. **Organization Relationships**:
   - An Organization can have a parent Organization (hierarchical structure)
   - An Organization contains multiple Ledgers
   - Organizations have addresses and legal information

2. **Ledger Relationships**:
   - A Ledger belongs to one Organization
   - A Ledger contains multiple Segments, Portfolios, and Assets
   - Ledgers are the primary financial record-keeping system

3. **Segment Relationships**:
   - A Segment belongs to an Organization and a Ledger
   - Segments can be associated with Accounts

4. **Portfolio Relationships**:
   - A Portfolio belongs to an Organization and a Ledger
   - A Portfolio is associated with an EntityID (likely a user or client)
   - Portfolios can contain multiple Accounts

5. **Account Relationships**:
   - An Account belongs to an Organization and a Ledger
   - An Account can be associated with a Portfolio and/or Segment
   - An Account is tied to a specific Asset (like a currency)
   - Accounts can have parent-child relationships (hierarchical)
   - Accounts have a type and can have an alias

6. **Asset Relationships**:
   - Assets belong to an Organization and Ledger
   - Assets have a code (e.g., "BRL" for Brazilian Real)
   - Assets are used by Accounts

## Transaction Component

The transaction component serves as the financial engine of the Midaz platform, handling all monetary movements between accounts.

### Transaction Technical Analysis

1. **Architecture**:
   - Follows the same microservice architecture as the onboarding component
   - Implemented in Go (Golang)
   - Uses a hexagonal/ports and adapters pattern with clear separation of:
     - Domain models (Transaction, Operation, etc.)
     - Repositories for data access
     - Services for business logic (command/query pattern)
     - HTTP adapters for API endpoints

2. **Transaction Processing**:
   - Uses a Domain Specific Language (DSL) through the "gold" package
   - Implements a parser for transaction definitions
   - Validates transactions before execution
   - Updates account balances atomically
   - Records individual operations for each account
   - Supports both simple transfers and complex multi-account transactions

3. **Data Storage & Access**:
   - PostgreSQL for transaction and operation data
   - Optimistic concurrency control through versioning
   - Repository pattern abstracts data access
   - Transactional guarantees for consistency

4. **Technical Features**:
   - Scale handling for different decimal places in currencies
   - Balance validation to prevent overdrafts
   - Support for different account types (including external accounts)
   - Double-entry accounting via debits and credits
   - Transaction reversal capabilities
   - Asynchronous processing via queues

5. **Observability**:
   - Detailed telemetry for transactions and operations
   - Error tracking and metrics collection
   - OpenTelemetry integration for tracing
   - Logging throughout the transaction lifecycle

### Transaction Business Analysis

1. **Core Business Function**:
   - Handles the movement of financial value between accounts
   - Maintains a complete audit trail through operations
   - Ensures financial integrity through double-entry accounting principles
   - Supports complex transaction scenarios through the DSL

2. **Transaction Types and Capabilities**:
   - Simple transfers between accounts
   - Multi-destination distributions (one-to-many)
   - Multi-source aggregations (many-to-one)
   - Percentage-based distributions
   - Transaction templates via chart of accounts grouping
   - Support for external account transactions (interfacing with outside systems)

3. **Business Rules Enforcement**:
   - Sufficient funds validation for debit operations
   - Asset code matching to prevent cross-currency issues
   - Permission controls for accounts (AllowSending, AllowReceiving)
   - Status-based restrictions on transactions
   - Support for pending transactions and approval flows

4. **Business Value**:
   - Enables financial applications with robust transaction capabilities
   - Maintains the integrity of financial data
   - Provides detailed audit trails for compliance
   - Flexible enough to support various business models
   - Scale handling to support different currencies and decimal requirements

### Transaction Entity Relationships

1. **Transaction Relationships**:
   - A Transaction belongs to an Organization and a Ledger (inherited from onboarding)
   - A Transaction can have a parent Transaction (hierarchical)
   - A Transaction contains multiple Operations
   - A Transaction is defined by a Send structure with Source and Distribute components
   - Transactions have a body that uses the Gold DSL to define the transaction logic

2. **Operation Relationships**:
   - An Operation belongs to a Transaction
   - An Operation affects a specific Account's Balance
   - Operations are either DEBIT or CREDIT type
   - Operations track balance before and after the transaction
   - Operations are associated with ChartOfAccounts for accounting categorization

3. **Balance Relationships**:
   - A Balance belongs to an Account
   - A Balance tracks Available and OnHold amounts for an Asset
   - Balances are versioned for concurrency control
   - Balances have settings that control sending/receiving permissions

4. **FromTo Relationships**:
   - FromTo can be Sources (From) or Destinations (To)
   - FromTo references Accounts by ID or Alias
   - FromTo can specify exact amounts or shares of the total
   - FromTo can include metadata and descriptions

## Infrastructure Component

The infrastructure component provides the foundational services that support both the onboarding and transaction components. It establishes a robust, scalable, and observable platform for the business components to operate on.

### Infrastructure Technical Analysis

1. **Containerization and Orchestration**:
   - Uses Docker for containerization of all services
   - Docker Compose for service orchestration and management
   - Creates isolated environments with defined resource allocations
   - Allows for consistent deployment across different environments

2. **High Availability and Resilience**:
   - PostgreSQL primary-replica setup for database redundancy
   - Health checks for all services to monitor and recover from failures
   - Restart policies to automatically recover from crashes
   - Volume persistence for data durability

3. **Security Architecture**:
   - All services configured with authentication
   - Network isolation through Docker networks
   - Environment variables for sensitive configuration (passwords, users)
   - No direct exposure of internal services

4. **Observability Architecture**:
   - Comprehensive monitoring with Grafana/OpenTelemetry
   - Tracing for distributed request flows
   - Metric collection for performance analysis
   - Log aggregation for troubleshooting

5. **Network Architecture**:
   - Isolated service-specific networks
   - Shared infra network for inter-component communication
   - Controlled exposure of ports to host system
   - Secure communication between services

### Infrastructure Services Overview

1. **Database Services**:
   - **PostgreSQL**: Primary-replica configuration for high availability
     - Persistent storage for structured data
     - Automated replication setup
     - Configurable performance parameters
   
   - **MongoDB**: Document database for flexible metadata
     - Authentication-secured access
     - Persistent volume for data durability

2. **Caching and Messaging**:
   - **Redis**: In-memory data store for caching and temporary storage
     - Password authentication
     - Persistent storage option
   
   - **RabbitMQ**: Message broker for asynchronous communication
     - Pre-configured users specific to each component
     - Defined queues and exchanges for business operations

3. **Observability Stack**:
   - **Grafana/OpenTelemetry** (OTEL-LGTM):
     - Grafana: Dashboards and visualization
     - Tempo: Distributed tracing
     - Prometheus: Metrics collection
     - OpenTelemetry Collector: Data collection and processing

### Infrastructure Integration Points

1. **Data Storage Integration**:
   - Business components connect to PostgreSQL and MongoDB for persistent storage
   - Database credentials and connection details passed via environment variables
   - Repository patterns in business components abstract the database implementation

2. **Messaging Integration**:
   - RabbitMQ enables asynchronous communication between components
   - Pre-defined queues for specific operations (e.g., transaction.balance_create.queue)
   - Component-specific users and permissions

3. **Caching Integration**:
   - Redis used by both components for caching and temporary data
   - Secure access through authentication
   - Configurable connection parameters

4. **Observability Integration**:
   - OpenTelemetry collects telemetry from all components
   - Custom dashboards in Grafana for business metrics
   - End-to-end tracing across components
   - Centralized logging for troubleshooting

5. **Network Integration**:
   - Shared `midaz_infra-network` for inter-component communication
   - Component-specific networks for isolation
   - Controlled port exposure for external access

## Integration Between Components

The onboarding, transaction, and infrastructure components work together to provide a complete financial platform:

1. **Structural Dependency**:
   - Transactions operate on the organizational structure created by the onboarding component
   - Accounts created in onboarding are the endpoints for transactions
   - Assets defined in onboarding are used for transaction validation
   - Both business components rely on infrastructure services

2. **Workflow Integration**:
   - Onboarding creates the financial structure (organizations, ledgers, accounts)
   - Transactions operate within that structure to move value
   - Both components share the same core domain model package (pkg/mmodel)
   - Infrastructure provides the foundational services for both components

3. **Shared Services**:
   - All components use the same infrastructure services:
     - PostgreSQL for primary data storage
     - MongoDB for metadata
     - RabbitMQ for messaging
     - Redis for caching
     - OpenTelemetry for observability

The architecture demonstrates a well-designed separation of concerns while maintaining cohesive integration between components, allowing the system to handle complex financial scenarios with robustness and flexibility. 