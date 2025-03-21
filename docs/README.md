# Midaz Documentation

Welcome to the Midaz documentation. This directory contains comprehensive documentation about the architecture and components of the Midaz platform developed by Lerian Studio.

## Available Documentation

- [Architecture Overview](architecture.md) - Comprehensive technical and business analysis of the onboarding and transaction components
- [Infrastructure Component](infrastructure.md) - Detailed documentation of the infrastructure services that support the platform
- [Entity Relationships](entity-relationships.md) - Visual diagrams of entity relationships within the platform
- [Telemetry Usage Examples](telemetry-usage-examples.md) - Examples and best practices for using the telemetry system

## Purpose

This documentation is intended to help developers understand the architecture and component relationships within the Midaz platform. It provides both technical and business perspectives to give a complete picture of how the system functions.

## Key Components

Midaz consists of three primary components:

1. **Infrastructure Component**: Provides foundational services like databases, message brokers, caching, and observability tools.

2. **Onboarding Component**: Handles the creation and management of organizational structures, ledgers, accounts, and assets.

3. **Transaction Component**: Processes financial transactions between accounts, providing robust double-entry accounting with comprehensive audit trails.

## Understanding the Entity Relationships

The entity relationship diagrams use the Mermaid syntax and should render automatically on GitHub or other Markdown viewers that support Mermaid. If they don't render properly, you can copy the diagram code and paste it into the [Mermaid Live Editor](https://mermaid.live/) to visualize them.

## Telemetry and Observability

Midaz includes a comprehensive telemetry system built on OpenTelemetry that provides metrics, traces, and logs for operational visibility. See the [Telemetry Usage Examples](telemetry-usage-examples.md) document for implementation examples and best practices.

## Infrastructure Services

The platform is supported by several infrastructure services that provide essential functionality:

- **PostgreSQL**: Primary-replica database setup for structured data storage
- **MongoDB**: Document database for flexible metadata storage
- **Redis**: In-memory data store for caching and temporary storage
- **RabbitMQ**: Message broker for asynchronous communication
- **Grafana/OpenTelemetry**: Comprehensive monitoring and observability stack

For detailed information on these services, see the [Infrastructure Component](infrastructure.md) documentation.

## Contributing to Documentation

When adding new features or making significant changes to existing components, please update this documentation accordingly. Keeping the documentation in sync with the codebase helps future developers understand the system more quickly. 