# Onboarding Bootstrap

## Overview

The `bootstrap` package provides application initialization and dependency injection for the Midaz onboarding service. It is responsible for wiring up all components and starting the service.

## Purpose

This package handles:

- **Configuration loading**: Environment variable parsing
- **Dependency initialization**: Database connections, message queues, caches
- **Repository creation**: PostgreSQL, MongoDB, RabbitMQ, Redis repositories
- **Use case wiring**: Dependency injection for command and query use cases
- **Handler creation**: HTTP request handlers
- **Middleware setup**: Authentication, telemetry, logging
- **Server startup**: HTTP server with graceful shutdown

## Package Structure

```
bootstrap/
├── config.go         # Configuration and dependency injection
├── service.go        # Service struct and Run method
├── fiber.server.go   # HTTP server configuration
└── README.md         # This file
```

## Key Components

### Config Struct

The `Config` struct contains all environment-based configuration:

```go
type Config struct {
    // Server
    ServerAddress string

    // PostgreSQL (primary and replica)
    PrimaryDBHost, PrimaryDBUser, PrimaryDBPassword, PrimaryDBName, PrimaryDBPort string
    ReplicaDBHost, ReplicaDBUser, ReplicaDBPassword, ReplicaDBName, ReplicaDBPort string

    // MongoDB
    MongoURI, MongoDBHost, MongoDBName, MongoDBUser, MongoDBPassword string

    // RabbitMQ
    RabbitMQHost, RabbitMQDefaultUser, RabbitMQDefaultPass, RabbitMQExchange string

    // Redis
    RedisHost, RedisPassword string

    // OpenTelemetry
    OtelServiceName, OtelColExporterEndpoint string
    EnableTelemetry bool

    // Authentication
    JWKAddress string
    AuthEnabled bool
}
```

### InitServers Function

The main bootstrap function that initializes the entire application:

**Initialization Order:**

1. Load configuration from environment
2. Initialize logger (Zap)
3. Initialize telemetry (OpenTelemetry)
4. Create PostgreSQL connection (primary + replica)
5. Create MongoDB connection
6. Create RabbitMQ connection
7. Create Redis connection
8. Initialize repositories:
   - Organization, Ledger, Account, Asset
   - Portfolio, Segment, AccountType
   - Metadata (MongoDB)
   - Producer (RabbitMQ)
   - Consumer (Redis)
9. Wire up use cases:
   - Command use case (write operations)
   - Query use case (read operations)
10. Create HTTP handlers
11. Configure authentication middleware
12. Create HTTP router
13. Create server instance
14. Return service ready to run

### Service Struct

The top-level application container:

```go
type Service struct {
    *Server       // HTTP server
    libLog.Logger // Application logger
}
```

**Methods:**

- `Run()`: Starts the application with graceful shutdown

### Server Struct

The HTTP server configuration:

```go
type Server struct {
    app           *fiber.App
    serverAddress string
    logger        libLog.Logger
    telemetry     libOpentelemetry.Telemetry
}
```

**Methods:**

- `ServerAddress()`: Returns the configured listen address
- `Run()`: Starts the HTTP server with graceful shutdown

## Dependency Injection Pattern

The bootstrap follows a clear dependency injection pattern:

```
┌─────────────────────────────────────────────────────────┐
│                     Environment                          │
│                   (Config from env vars)                 │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                    Infrastructure                        │
│  PostgreSQL │ MongoDB │ RabbitMQ │ Redis │ Telemetry   │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                     Repositories                         │
│  Organization │ Ledger │ Account │ Asset │ Metadata    │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                      Use Cases                           │
│            Command Use Case │ Query Use Case             │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                     HTTP Handlers                        │
│  Organization │ Ledger │ Account │ Asset │ Portfolio   │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                      HTTP Router                         │
│              (Fiber App with Middleware)                 │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                      HTTP Server                         │
│                 (Listening on :3000)                     │
└─────────────────────────────────────────────────────────┘
```

## Configuration

### Environment Variables

**Server:**

- `SERVER_ADDRESS`: HTTP server listen address (default: ":3000")

**PostgreSQL Primary:**

- `DB_HOST`: Primary database host
- `DB_USER`: Primary database user
- `DB_PASSWORD`: Primary database password
- `DB_NAME`: Primary database name
- `DB_PORT`: Primary database port
- `DB_SSLMODE`: SSL mode (disable, require, verify-ca, verify-full)
- `DB_MAX_OPEN_CONNS`: Maximum open connections
- `DB_MAX_IDLE_CONNS`: Maximum idle connections

**PostgreSQL Replica:**

- `DB_REPLICA_HOST`: Replica database host
- `DB_REPLICA_USER`: Replica database user
- `DB_REPLICA_PASSWORD`: Replica database password
- `DB_REPLICA_NAME`: Replica database name
- `DB_REPLICA_PORT`: Replica database port
- `DB_REPLICA_SSLMODE`: Replica SSL mode

**MongoDB:**

- `MONGO_URI`: MongoDB protocol (mongodb, mongodb+srv)
- `MONGO_HOST`: MongoDB host
- `MONGO_NAME`: MongoDB database name
- `MONGO_USER`: MongoDB user
- `MONGO_PASSWORD`: MongoDB password
- `MONGO_PORT`: MongoDB port
- `MONGO_PARAMETERS`: Additional connection parameters
- `MONGO_MAX_POOL_SIZE`: Maximum connection pool size (default: 100)

**RabbitMQ:**

- `RABBITMQ_URI`: RabbitMQ protocol (amqp, amqps)
- `RABBITMQ_HOST`: RabbitMQ host
- `RABBITMQ_PORT_HOST`: RabbitMQ management port
- `RABBITMQ_PORT_AMQP`: RabbitMQ AMQP port
- `RABBITMQ_DEFAULT_USER`: RabbitMQ user
- `RABBITMQ_DEFAULT_PASS`: RabbitMQ password
- `RABBITMQ_EXCHANGE`: Exchange name for publishing
- `RABBITMQ_KEY`: Routing key for messages
- `RABBITMQ_HEALTH_CHECK_URL`: Health check endpoint

**Redis:**

- `REDIS_HOST`: Redis host(s) (comma-separated for cluster)
- `REDIS_MASTER_NAME`: Sentinel master name (for Redis Sentinel)
- `REDIS_PASSWORD`: Redis password
- `REDIS_DB`: Redis database number (default: 0)
- `REDIS_TLS`: Enable TLS (default: false)
- `REDIS_CA_CERT`: CA certificate for TLS
- `REDIS_USE_GCP_IAM`: Use GCP IAM authentication (default: false)
- `REDIS_POOL_SIZE`: Connection pool size (default: 10)
- `REDIS_READ_TIMEOUT`: Read timeout in seconds (default: 3)
- `REDIS_WRITE_TIMEOUT`: Write timeout in seconds (default: 3)

**OpenTelemetry:**

- `OTEL_RESOURCE_SERVICE_NAME`: Service name for traces
- `OTEL_LIBRARY_NAME`: Library name for instrumentation
- `OTEL_RESOURCE_SERVICE_VERSION`: Service version
- `OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT`: Environment (dev, staging, prod)
- `OTEL_EXPORTER_OTLP_ENDPOINT`: OpenTelemetry collector endpoint
- `ENABLE_TELEMETRY`: Enable/disable telemetry (default: true)

**Authentication:**

- `CASDOOR_JWK_ADDRESS`: Casdoor JWK endpoint for JWT validation
- `PLUGIN_AUTH_ENABLED`: Enable/disable authentication (default: false)
- `PLUGIN_AUTH_HOST`: Authentication service host

## Usage

### Starting the Service

```go
package main

import (
    "github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap"
)

func main() {
    service := bootstrap.InitServers()
    service.Run()
}
```

### Configuration Example

```bash
# Server
export SERVER_ADDRESS=":3000"

# PostgreSQL
export DB_HOST="localhost"
export DB_USER="midaz"
export DB_PASSWORD="secret"
export DB_NAME="midaz_onboarding"
export DB_PORT="5432"
export DB_SSLMODE="disable"

# MongoDB
export MONGO_URI="mongodb"
export MONGO_HOST="localhost"
export MONGO_NAME="midaz_metadata"
export MONGO_USER="midaz"
export MONGO_PASSWORD="secret"
export MONGO_PORT="27017"

# RabbitMQ
export RABBITMQ_URI="amqp"
export RABBITMQ_HOST="localhost"
export RABBITMQ_DEFAULT_USER="guest"
export RABBITMQ_DEFAULT_PASS="guest"
export RABBITMQ_EXCHANGE="midaz.exchange"

# Redis
export REDIS_HOST="localhost:6379"
export REDIS_PASSWORD=""
export REDIS_DB="0"

# OpenTelemetry
export OTEL_RESOURCE_SERVICE_NAME="onboarding"
export ENABLE_TELEMETRY="true"
```

## Graceful Shutdown

The service supports graceful shutdown, which:

1. Stops accepting new requests
2. Waits for in-flight requests to complete (with timeout)
3. Closes database connections
4. Flushes telemetry data
5. Closes RabbitMQ connections
6. Exits cleanly

Shutdown is triggered by:

- SIGTERM signal (Kubernetes, Docker)
- SIGINT signal (Ctrl+C)
- Application errors

## Error Handling

The bootstrap panics on critical initialization failures:

- Configuration loading errors
- Database connection failures
- RabbitMQ connection failures

This is intentional as the service cannot function without these dependencies.
The panic will be caught by the container orchestrator (Kubernetes) which will
restart the service.

## Dependencies

### External Services

The onboarding service requires:

- **PostgreSQL**: Primary entity storage (required)
- **MongoDB**: Metadata storage (required)
- **RabbitMQ**: Async messaging (required for account creation)
- **Redis**: Caching and idempotency (optional)
- **OpenTelemetry Collector**: Observability (optional)
- **Casdoor**: Authentication (optional)

### Connection Pooling

**PostgreSQL:**

- Primary connection for writes
- Replica connection for reads (if configured)
- Configurable pool size

**MongoDB:**

- Default max pool size: 100
- Configurable via MONGO_MAX_POOL_SIZE

**Redis:**

- Default pool size: 10
- Configurable via REDIS_POOL_SIZE

## Health Checks

The bootstrap initializes health check capabilities:

- RabbitMQ health check via `CheckRabbitMQHealth()`
- Database connection validation
- Used by HTTP health endpoints

## Telemetry

The service automatically instruments:

- HTTP requests (via Fiber middleware)
- Database queries (via lib-commons)
- RabbitMQ messages (via lib-commons)
- Business operations (via manual spans)

Traces are exported to the configured OpenTelemetry collector.

## Related Packages

- `internal/services`: Business logic layer
- `internal/adapters`: Infrastructure layer
- `internal/adapters/http/in`: HTTP handlers and routes

## Notes

- The bootstrap uses panic for critical failures (intentional fail-fast behavior)
- All connections are validated at startup
- The service won't start if any required dependency is unavailable
- Configuration errors are reported immediately at startup
