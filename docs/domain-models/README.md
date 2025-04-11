# Domain Models

**Navigation:** [Home](../) > Domain Models

This section provides detailed documentation about the domain models used throughout the Midaz platform, including entities, relationships, and architectural patterns.

## Overview

Domain models are the core abstractions that represent the financial concepts within the Midaz platform. These models define:

- The key business entities and their attributes
- Relationships between different entities
- Business rules and constraints
- Behaviors and operations

Understanding these domain models is essential for effective use of the Midaz platform and for building applications that integrate with it.

## Core Domain Concepts

### Financial Entity Hierarchy

Midaz uses a hierarchical structure for organizing financial entities:

- **Organizations**: Top-level container entities (e.g., companies, institutions)
- **Ledgers**: Financial books within an organization 
- **Assets**: Currencies or financial instruments (e.g., USD, EUR, BTC)
- **Segments**: Business segments for categorization
- **Portfolios**: Collections of related accounts
- **Accounts**: Individual financial accounts that can hold balances

Each of these entities follows a consistent pattern with:
- Unique identifiers
- Core attributes
- Extensible metadata
- Relationships to other entities

For more details, see the [Entity Hierarchy](./entity-hierarchy.md) documentation.

### Financial Transactions

Transactions represent the movement of value between accounts and follow double-entry bookkeeping principles:

- Each transaction consists of operations
- The sum of all operations in a transaction must equal zero
- Operations can be debits (negative) or credits (positive)
- Transactions update account balances

For more details, see the [Financial Model](./financial-model.md) documentation.

### Metadata Approach

All entities in Midaz support extensible metadata, which allows for flexible customization without changing the core schema:

- Metadata is stored as key-value pairs
- Keys and values are strings with size limits
- Metadata can be used for filtering and categorization
- Metadata can be updated independently from the main entity attributes

For more details, see the [Metadata Approach](./metadata-approach.md) documentation.

## Domain Model Documentation

- [Entity Hierarchy](./entity-hierarchy.md) - The structure and relationships of financial entities
- [Financial Model](./financial-model.md) - Double-entry bookkeeping and transaction model
- [Metadata Approach](./metadata-approach.md) - Extending entities with custom attributes

## Domain-Driven Design

Midaz follows Domain-Driven Design (DDD) principles to ensure that the software model accurately reflects the financial domain:

- **Bounded Contexts**: Clear boundaries between different parts of the system
- **Ubiquitous Language**: Consistent terminology throughout the codebase and documentation
- **Aggregates**: Cluster of domain objects treated as a unit
- **Repositories**: Abstraction for data persistence
- **Domain Services**: Operations that don't belong to any specific entity
- **Value Objects**: Immutable objects that represent concepts with no identity

## Data Models vs. Domain Models

It's important to understand the distinction:

- **Data Models**: Focus on storage representation (database tables, fields)
- **Domain Models**: Focus on business concepts and behaviors (entities, relationships, rules)

Midaz uses a clear separation between these concerns, with repositories and adapters translating between domain models and data storage.

## Related Documentation

- [API Reference](../api-reference/README.md) - API endpoints for working with domain entities
- [Architecture](../architecture/README.md) - The system architecture that implements these domain models
- [Tutorials](../tutorials/README.md) - Practical examples of working with domain models