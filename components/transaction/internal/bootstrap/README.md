# Bootstrap - Transaction Service

This package handles dependency injection and service initialization for the transaction service.

## Components

### config.go

Environment configuration loading and validation. Reads from environment variables and `.env` files to configure:

- Database connections (PostgreSQL, MongoDB)
- Message broker (RabbitMQ)
- Cache (Redis)
- HTTP server settings
- Observability (tracing, metrics)
- Async processing settings

### fiber.server.go

HTTP server setup using the Fiber framework:

- Middleware configuration (CORS, logging, tracing)
- Route registration
- Health check endpoints
- Swagger documentation serving
- Graceful shutdown handling

### rabbitmq.server.go

Message queue consumer setup:

- Queue configuration and binding
- Consumer worker pool management
- Error handling and retry logic
- Dead letter queue setup

### redis.consumer.go

Redis queue consumer for backup transaction processing:

- Polls backup queue for failed transactions
- Implements retry logic with exponential backoff
- Ensures eventual consistency

### service.go

Main service orchestrator that:

- Assembles all components
- Manages service lifecycle
- Coordinates multiple consumers (HTTP, RabbitMQ, Redis)
- Handles graceful shutdown across all components

## Initialization Flow

1. **Environment Loading** - Parse configuration from environment
2. **Infrastructure Setup** - Initialize database connections, message brokers, cache
3. **Repository Creation** - Instantiate PostgreSQL, MongoDB, Redis repositories
4. **Use Case Wiring** - Inject repositories into command/query use cases
5. **Handler Setup** - Create HTTP handlers and queue consumers
6. **Multi-Service Start** - Begin HTTP server, RabbitMQ consumer, and Redis consumer

## Dependency Graph

```
Service
  ├── HTTP Server
  │   └── Handlers → Command/Query UseCases
  ├── RabbitMQ Consumer
  │   └── Message Processor → Command UseCases
  └── Redis Queue Consumer
      └── Retry Processor → Command UseCases
          │
          └── Shared Dependencies:
              ├── PostgreSQL Repositories
              ├── MongoDB Repository
              ├── RabbitMQ Producer
              └── Redis Cache + Lua Scripts
```

## Async Processing

The service supports both synchronous and asynchronous transaction processing:

- **Synchronous** - Direct HTTP request/response
- **Asynchronous** - Via RabbitMQ for high-throughput scenarios
- **Retry Queue** - Redis backup queue for failed async operations
