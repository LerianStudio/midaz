# Architecture

**Navigation:** [Home](../) > Architecture

This section provides comprehensive documentation on the Midaz platform architecture, design principles, and system organization.

## Overview

Midaz is built using modern architectural patterns to ensure scalability, maintainability, and extensibility. The system follows a microservices architecture with clear boundaries between components, each responsible for specific business domains.

The platform is designed around these key architectural principles:

- **Hexagonal Architecture**: Clean separation of domains, application logic, and infrastructure
- **Event-Driven Design**: Asynchronous communication between services via events
- **CQRS (Command Query Responsibility Segregation)**: Separation of read and write operations
- **Domain-Driven Design**: Business domains modeled as bounded contexts

## Core Architectural Components

The Midaz platform consists of several key components:

- **Onboarding Service**: Manages the entity hierarchy (organizations, ledgers, accounts, etc.)
- **Transaction Service**: Handles financial transactions and balance management
- **MDZ CLI**: Command-line interface for interacting with Midaz services
- **Infrastructure Components**: Databases, message brokers, and other shared infrastructure

These components interact through well-defined APIs and event streams, following the principles of loose coupling and high cohesion.

## Architecture Documentation

Detailed documentation on different aspects of the architecture:

- [System Overview](./system-overview.md) - High-level view of the entire system
- [Hexagonal Architecture](./hexagonal-architecture.md) - Details on the ports and adapters pattern
- [Event-Driven Design](./event-driven-design.md) - Event-based communication patterns
- [Component Integration](./component-integration.md) - How components interact
- [Data Flow](./data-flow/) - Information flow through the system

## Architecture Diagrams

The architecture documentation includes several key diagrams:

- **System Context Diagram**: Shows the Midaz system in its environment
- **Container Diagram**: Depicts the high-level components
- **Component Diagrams**: Details the internal structure of each component
- **Data Flow Diagrams**: Illustrates how data moves through the system
- **Sequence Diagrams**: Shows interaction patterns for key processes

## Key Design Decisions

### Service Boundaries

Services are divided along domain boundaries, with each service responsible for a specific aspect of the financial system:

- **Onboarding Service**: Entity management and metadata
- **Transaction Service**: Transaction processing and balance management

### Data Persistence

- **PostgreSQL**: Primary database for transactional data
- **MongoDB**: Document store for metadata and flexible attributes
- **Redis**: Caching and distributed locks

### Communication Patterns

- **Synchronous**: RESTful HTTP APIs for direct client interactions
- **Asynchronous**: RabbitMQ for event-based communication between services

## Technical Architecture

Midaz is built with the following technologies:

- **Backend Services**: Go language with Fiber web framework
- **API Documentation**: OpenAPI (Swagger)
- **Authentication**: OAuth 2.0 through a pluggable auth provider
- **Infrastructure**: Docker and Docker Compose for containerization
- **Observability**: Grafana, Prometheus, and OpenTelemetry

## Next Steps

To dive deeper into the architecture, start with the [System Overview](./system-overview.md) and then explore the specific aspects you're interested in. For a practical understanding, the [Quickstart Guide](../getting-started/quickstart.md) demonstrates how the architecture supports common workflows.