# Transaction Bootstrap

## Overview

The `bootstrap` package provides application initialization and dependency injection for the Midaz transaction service. It is responsible for wiring up all components and starting multiple concurrent servers.

## Purpose

This package handles:

- **Configuration loading**: Environment variable parsing
- **Dependency initialization**: Database connections, message queues, caches
- **Repository creation**: PostgreSQL, MongoDB, RabbitMQ, Redis repositories
- **Use case wiring**: Dependency injection for command and query use cases
- **Handler creation**: HTTP request handlers
- **Middleware setup**: Authentication, telemetry, logging
- **Multi-server startup**: HTTP, RabbitMQ consumer, Redis consumer

## Package Structure

```
bootstrap/
├── config.go           # Configuration and dependency injection
├── service.go          # Service struct and main Run method
├── fiber.server.go     # HTTP server component
├── rabbitmq.server.go  # RabbitMQ consumer component
├── redis.consumer.go   # Redis queue consumer component
└── README.md           # This file
```

## Components

### 1. Service (service.go)

The top-level application container that coordinates all components:

```go
type Service struct {
    *Server                 // HTTP server
    *MultiQueueConsumer     // RabbitMQ consumer
    *RedisQueueConsumer     // Redis consumer
    libLog.Logger           // Application logger
}
```

**Run Method:**

- Starts all components concurrently using lib-commons Launcher
- Handles graceful shutdown on SIGTERM/SIGINT
- Waits for all components to shut down cleanly

### 2. HTTP Server (fiber.server.go)

Handles REST API requests:

```go
type Server struct {
    app           *fiber.App
    serverAddress string
    logger        libLog.Logger
    telemetry     libOpentelemetry.Telemetry
}
```

**Features:**

- Transaction creation and management
- Balance queries
- Operation tracking
- Asset rate management
- Transaction routing configuration
- Graceful shutdown support

### 3. RabbitMQ Consumer (rabbitmq.server.go)

Processes asynchronous messages from RabbitMQ:

```go
type MultiQueueConsumer struct {
    consumerRoutes *rabbitmq.ConsumerRoutes
    UseCase        *command.UseCase
}
```

**Queues Processed:**

1. **Balance Create Queue** (`RABBITMQ_BALANCE_CREATE_QUEUE`)

   - Processes account creation events from onboarding service
   - Creates initial balance entries for new accounts
   - JSON message format

2. **BTO Queue** (`RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE`)
   - Processes balance/transaction/operation updates
   - Handles async transaction execution
   - Msgpack message format

**Configuration:**

- `RABBITMQ_NUMBERS_OF_WORKERS`: Workers per queue (parallel processing)
- `RABBITMQ_NUMBERS_OF_PREFETCH`: Prefetch count for throughput optimization

### 4. Redis Queue Consumer (redis.consumer.go)

Processes stale transaction messages from Redis (recovery mechanism):

```go
type RedisQueueConsumer struct {
    Logger             libLog.Logger
    TransactionHandler in.TransactionHandler
}
```

**Features:**

- **Cron-based processing**: Runs every 30 minutes
- **Age filtering**: Processes messages older than 30 minutes
- **Worker pool**: Up to 100 concurrent workers
- **Recovery mechanism**: Ensures eventual consistency

**Constants:**

- `CronTimeToRun`: 30 minutes (processing interval)
- `MessageTimeOfLife`: 30 minutes (age threshold)
- `MaxWorkers`: 100 (max concurrent workers)

**Processing Logic:**

1. Reads all messages from Redis queue
2. Filters messages by age (> 30 minutes)
3. Processes eligible messages in parallel
4. Skips recent messages (still in primary flow)
5. Handles graceful shutdown

## Dependency Injection

The `config.go` file implements comprehensive dependency injection:

```
Config → Connections → Repositories → Use Cases → Handlers → Router → Servers
```

### Initialization Flow

1. **Load Configuration**

   - Parse environment variables
   - Validate required settings

2. **Initialize Infrastructure**

   - PostgreSQL connections (primary + replica)
   - MongoDB connection
   - Redis connection (with Sentinel/IAM support)
   - RabbitMQ connection (producer + consumer)

3. **Create Repositories**

   - PostgreSQL: Transaction, Balance, Operation, AssetRate, Routes
   - MongoDB: Metadata
   - RabbitMQ: Producer, Consumer
   - Redis: Cache operations

4. **Wire Use Cases**

   - Command use case (write operations)
   - Query use case (read operations)

5. **Create Handlers**

   - HTTP handlers for REST API
   - Consumer handlers for queues

6. **Configure Middleware**

   - Authentication (Casdoor/JWT)
   - Telemetry (OpenTelemetry)
   - Logging
   - Error handling

7. **Setup Routing**

   - REST API routes
   - Swagger documentation

8. **Create Servers**
   - HTTP server
   - RabbitMQ consumer
   - Redis consumer

## Configuration

### Environment Variables

**Database (PostgreSQL):**

- `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT`, `DB_SSLMODE`
- `DB_REPLICA_HOST`, `DB_REPLICA_USER`, `DB_REPLICA_PASSWORD`, `DB_REPLICA_NAME`, `DB_REPLICA_PORT`, `DB_REPLICA_SSLMODE`
- `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`

**Database (MongoDB):**

- `MONGO_URI` or `MONGO_HOST`, `MONGO_USER`, `MONGO_PASSWORD`, `MONGO_NAME`, `MONGO_PORT`
- `MONGO_PARAMETERS`, `MONGO_MAX_POOL_SIZE`

**Cache (Redis):**

- `REDIS_HOST`, `REDIS_PASSWORD`, `REDIS_DB`
- `REDIS_MASTER_NAME` (for Sentinel)
- `REDIS_USE_GCP_IAM`, `REDIS_SERVICE_ACCOUNT` (for GCP)
- `REDIS_POOL_SIZE`, `REDIS_MIN_IDLE_CONNS`
- `REDIS_READ_TIMEOUT`, `REDIS_WRITE_TIMEOUT`

**Message Queue (RabbitMQ):**

- `RABBITMQ_URI` or `RABBITMQ_HOST`, `RABBITMQ_PORT_HOST`, `RABBITMQ_PORT_AMQP`
- `RABBITMQ_DEFAULT_USER`, `RABBITMQ_DEFAULT_PASS`
- `RABBITMQ_CONSUMER_USER`, `RABBITMQ_CONSUMER_PASS`
- `RABBITMQ_BALANCE_CREATE_QUEUE`
- `RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE`
- `RABBITMQ_NUMBERS_OF_WORKERS`, `RABBITMQ_NUMBERS_OF_PREFETCH`

**Authentication:**

- `CASDOOR_ADDRESS`, `CASDOOR_CLIENT_ID`, `CASDOOR_CLIENT_SECRET`
- `CASDOOR_ORGANIZATION_NAME`, `CASDOOR_APPLICATION_NAME`
- `CASDOOR_JWK_ADDRESS`

**Telemetry:**

- `ENABLE_TELEMETRY`
- `OTEL_RESOURCE_SERVICE_NAME`, `OTEL_LIBRARY_NAME`
- `OTEL_RESOURCE_SERVICE_VERSION`, `OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT`
- `OTEL_EXPORTER_OTLP_ENDPOINT`

**Server:**

- `SERVER_ADDRESS` (e.g., ":3000")
- `ENV_NAME`, `LOG_LEVEL`

## Architecture

The bootstrap package follows hexagonal architecture principles:

```
┌─────────────────────────────────────────────────────────┐
│                    Bootstrap Layer                       │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │              Config & DI                          │  │
│  │  - Load environment variables                     │  │
│  │  - Create connections                             │  │
│  │  - Wire dependencies                              │  │
│  └──────────────────────────────────────────────────┘  │
│                         │                                │
│  ┌──────────────────────────────────────────────────┐  │
│  │         Multi-Server Coordination                 │  │
│  │                                                    │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌─────────┐│  │
│  │  │ HTTP Server  │  │   RabbitMQ   │  │  Redis  ││  │
│  │  │              │  │   Consumer   │  │Consumer ││  │
│  │  │ REST API     │  │              │  │         ││  │
│  │  │ Requests     │  │ Async Queue  │  │ Stale   ││  │
│  │  │              │  │ Processing   │  │Messages ││  │
│  │  └──────────────┘  └──────────────┘  └─────────┘│  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Usage

### Starting the Service

```go
package main

import "github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"

func main() {
    service := bootstrap.InitServers()
    service.Run()
}
```

### Graceful Shutdown

All components support graceful shutdown:

1. SIGTERM/SIGINT signal received
2. HTTP server stops accepting new requests
3. In-flight HTTP requests complete
4. RabbitMQ consumers finish current messages
5. Redis consumer completes current batch
6. Database connections closed
7. Telemetry flushed
8. Application exits

## Multi-Server Architecture

The transaction service runs three concurrent servers:

1. **HTTP Server**: Synchronous REST API
2. **RabbitMQ Consumer**: Asynchronous message processing
3. **Redis Consumer**: Stale message recovery

All servers:

- Run concurrently in separate goroutines
- Share the same database connections and repositories
- Use the same logger and telemetry
- Shut down gracefully together

## Best Practices

1. **Configuration Management**

   - Use environment variables for all configuration
   - Provide sensible defaults where appropriate
   - Validate required configuration at startup

2. **Connection Management**

   - Reuse connections across all components
   - Configure connection pools appropriately
   - Handle connection failures gracefully

3. **Error Handling**

   - Panic on critical startup failures (database connections)
   - Log and continue on non-critical failures
   - Return errors from message handlers for requeue

4. **Resource Cleanup**
   - Use defer for cleanup in handlers
   - Implement graceful shutdown for all components
   - Close all connections on shutdown

## Related Packages

- `services/command`: Business logic for write operations
- `services/query`: Business logic for read operations
- `adapters/`: Infrastructure implementations
- `adapters/http/in`: HTTP handlers
