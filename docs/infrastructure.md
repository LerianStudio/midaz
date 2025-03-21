# Midaz Infrastructure Component

The infrastructure component of Midaz provides the foundational services and resources that support the platform's business components (onboarding and transaction). It is designed with high availability, security, and observability in mind.

## Overview

The infrastructure component is responsible for provisioning, configuring, and managing the underlying services required by the business components. It uses Docker for containerization and orchestration, ensuring consistent and reproducible environments.

## Key Services

### Database Services

#### PostgreSQL

- **Architecture**: Primary-replica configuration for high availability
- **Purpose**: Persistent storage for structured data
- **Features**:
  - Automated replication setup
  - Configurable performance parameters (connections, buffers)
  - Health checks and automatic recovery
  - Custom initialization scripts
- **Integration**: Used by both onboarding and transaction components

#### MongoDB

- **Purpose**: Document storage, primarily for flexible metadata
- **Features**:
  - Authentication-secured access
  - Persistent volume for data durability
- **Integration**: Used by both components for storing unstructured metadata

### Caching and Messaging

#### Redis

- **Purpose**: In-memory data store for caching and temporary storage
- **Features**:
  - Password authentication
  - Persistent storage option
- **Integration**: Used for high-speed data access and caching

#### RabbitMQ

- **Purpose**: Message broker for asynchronous communication
- **Features**:
  - Pre-configured users specific to each component
  - Defined queues and exchanges for business operations
  - Management interface for monitoring
- **Integration**: Enables event-driven architecture between components
- **Key Queues**:
  - `transaction.balance_create.queue`
  - `transaction.transaction_balance_operation.queue`

### Observability Stack

#### Grafana/OpenTelemetry (OTEL-LGTM)

- **Purpose**: Comprehensive monitoring and observability
- **Components**:
  - Grafana: Dashboards and visualization
  - Tempo: Distributed tracing
  - Prometheus: Metrics collection
  - OpenTelemetry Collector: Data collection and processing
- **Features**:
  - Custom dashboards for business metrics
  - Trace correlation between components
  - Log aggregation and analysis
  - Alerting capabilities
- **Integration**: Collects telemetry from all business components

## Technical Architecture

### Containerization and Orchestration

- Docker containers for all services
- Docker Compose for service definition and orchestration
- Named volumes for data persistence
- Health checks for service monitoring

### Network Architecture

- Isolated service-specific networks
- Shared `midaz_infra-network` for inter-component communication
- Controlled exposure of ports to host system
- Secure communication between services

### High Availability and Resilience

- Database replication for fault tolerance
- Automatic restart policies for all services
- Health monitoring and recovery
- Volume persistence for data durability

### Security

- All services configured with authentication
- Network isolation through Docker networks
- Environment variables for sensitive configuration
- No direct exposure of internal services

### Configuration Management

- Environment variables for service configuration
- Template .env files for environment setup
- Docker volumes for persistent configuration
- Initialization scripts for service setup

## Integration with Business Components

The infrastructure component integrates with the business components (onboarding and transaction) in several ways:

1. **Data Storage**: Provides PostgreSQL and MongoDB databases used by business components
2. **Messaging**: Enables asynchronous communication through RabbitMQ
3. **Caching**: Provides Redis for high-speed data access
4. **Observability**: Collects and visualizes telemetry data from all components
5. **Networking**: Establishes secure communication channels between components

## Management

The infrastructure component can be managed using the Makefile commands from the project root:

```bash
# Get information about available commands
make infra COMMAND="info"

# Start all infrastructure services
make infra COMMAND="up"

# Stop all infrastructure services
make infra COMMAND="down"

# View logs from all services
make infra COMMAND="logs"
```

## Design Principles

1. **Infrastructure as Code**: All infrastructure defined in configuration files
2. **High Availability**: Designed for fault tolerance and resilience
3. **Scalability**: Can be scaled to handle increased load
4. **Security**: Follows security best practices
5. **Observability**: Comprehensive monitoring and troubleshooting
6. **DevOps-Friendly**: Easy to manage and operate

## Conclusion

The infrastructure component forms the foundation of the Midaz platform, providing the essential services required by the business components. Its well-designed architecture ensures that the platform is reliable, secure, and observable, enabling the business components to focus on their specific domains without worrying about the underlying infrastructure. 