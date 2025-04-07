# Infrastructure Components

**Navigation:** [Home](../../) > [Components](../) > Infrastructure

## Overview

The Midaz infrastructure layer provides the foundational services required for the operation of the Midaz system. It employs a containerized architecture with Docker Compose for consistent deployment across environments.

## Infrastructure Components

The infrastructure layer consists of the following components:

### PostgreSQL Database

PostgreSQL is used as the primary relational database for structured data.

**Key Features:**
- Primary/replica configuration for high availability
- Dedicated databases for each service (onboarding, transaction)
- Logical replication for data redundancy
- Migration scripts for schema versioning
- Performance-tuned configuration

**Related Documentation:**
- [PostgreSQL Configuration](./postgresql.md)

### MongoDB

MongoDB is used as a document database for flexible metadata storage.

**Key Features:**
- Replica set configuration for redundancy
- Secured with authentication and keyfile
- Namespace separation for different services
- Schema-less design for flexible metadata storage

**Related Documentation:**
- [MongoDB Configuration](./mongodb.md)

### RabbitMQ

RabbitMQ serves as the message broker for asynchronous communication between services.

**Key Features:**
- Predefined exchanges and queues for transaction processing
- Direct exchange types for message routing
- Durable queues for message persistence
- Bindings for queue-to-exchange relationships
- Web management interface for monitoring

**Related Documentation:**
- [RabbitMQ Configuration](./rabbitmq.md)

### Redis/Valkey

Redis (Valkey) is used as an in-memory data store for caching and temporary data.

**Key Features:**
- Password-protected access
- Persistence configuration for data durability
- Used for caching and session management
- Low-latency data access

**Related Documentation:**
- [Redis/Valkey Configuration](./redis.md)

### Grafana/OpenTelemetry

Monitoring and observability are provided through Grafana and OpenTelemetry.

**Key Features:**
- Integrated logging, tracing, and metrics collection
- Pre-configured dashboards for system monitoring
- Customizable visualization for system metrics
- Secure access controls

**Related Documentation:**
- [Monitoring Setup](./monitoring.md)

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│                   Infrastructure Layer                   │
├─────────────────┬─────────────────┬─────────────────────┤
│                 │                 │                     │
│   PostgreSQL    │    MongoDB      │     RabbitMQ        │
│ ┌─────────────┐ │ ┌─────────────┐ │  ┌─────────────┐    │
│ │  Primary    │ │ │  Replica    │ │  │ Exchanges   │    │
│ │  Database   │ │ │  Set (rs0)  │ │  │ & Queues    │    │
│ └──────┬──────┘ │ └─────────────┘ │  └─────────────┘    │
│        │        │                 │                     │
│ ┌──────┴──────┐ │                 │                     │
│ │  Replica    │ │                 │                     │
│ │  Database   │ │                 │                     │
│ └─────────────┘ │                 │                     │
├─────────────────┼─────────────────┼─────────────────────┤
│                 │                 │                     │
│   Redis/Valkey  │    Grafana      │  OpenTelemetry      │
│ ┌─────────────┐ │ ┌─────────────┐ │  ┌─────────────┐    │
│ │  In-Memory  │ │ │ Dashboards  │ │  │ Collectors  │    │
│ │  Data Store │ │ │ & Metrics   │ │  │ & Exporters │    │
│ └─────────────┘ │ └─────────────┘ │  └─────────────┘    │
│                 │                 │                     │
└─────────────────┴─────────────────┴─────────────────────┘
```

## Network Configuration

The infrastructure components are organized into a dedicated network for isolation and security:

- `infra-network`: Core infrastructure components
- Service-specific networks: For connection to application services

## Management Commands

Infrastructure components can be managed through the root Makefile:

```bash
# Start all infrastructure components
make up

# Stop all infrastructure components
make down

# View logs from infrastructure components
make logs

# Clean up infrastructure resources
make clean-docker

# Rebuild and restart infrastructure components
make rebuild-up
```

## Environment Configuration

Each infrastructure component is configured through environment variables defined in `.env` files. These files are created from the provided `.env.example` templates:

```bash
# Set up environment configuration
make set-env
```

Key environment variables include:
- Database credentials and connection settings
- Message broker configuration
- Monitoring system access controls
- Network ports and bindings

## Docker Compose Configuration

The infrastructure is defined in `docker-compose.yml` files, with the main file located at `/components/infra/docker-compose.yml`. This file orchestrates:

- Container definitions and relationships
- Volume mounting for data persistence
- Network configuration and exposure
- Health checks and dependent services

## Next Steps

To learn more about individual infrastructure components:

- [PostgreSQL Configuration](./postgresql.md)
- [MongoDB Configuration](./mongodb.md)
- [RabbitMQ Configuration](./rabbitmq.md)
- [Redis/Valkey Configuration](./redis.md)
- [Monitoring Setup](./monitoring.md)