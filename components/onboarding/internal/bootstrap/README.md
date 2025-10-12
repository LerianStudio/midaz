# Bootstrap - Onboarding Service

This package handles dependency injection and service initialization for the onboarding service.

## Components

### config.go

Environment configuration loading and validation. Reads from environment variables and `.env` files to configure:

- Database connections (PostgreSQL, MongoDB)
- Message broker (RabbitMQ)
- Cache (Redis)
- HTTP server settings
- Observability (tracing, metrics)

### fiber.server.go

HTTP server setup using the Fiber framework:

- Middleware configuration (CORS, logging, tracing)
- Route registration
- Health check endpoints
- Swagger documentation serving
- Graceful shutdown handling

### service.go

Main service orchestrator that:

- Assembles all components
- Manages service lifecycle
- Coordinates graceful startup/shutdown

## Initialization Flow

1. **Environment Loading** - Parse configuration from environment
2. **Infrastructure Setup** - Initialize database connections, message brokers
3. **Repository Creation** - Instantiate PostgreSQL, MongoDB, Redis repositories
4. **Use Case Wiring** - Inject repositories into command/query use cases
5. **Handler Setup** - Create HTTP handlers with use case dependencies
6. **Server Start** - Begin accepting HTTP requests

## Dependency Graph

```
Service
  └── Server (HTTP)
      ├── Handlers
      │   ├── Command UseCase
      │   │   ├── PostgreSQL Repositories
      │   │   ├── MongoDB Repository
      │   │   ├── RabbitMQ Producer
      │   │   └── Redis Cache
      │   └── Query UseCase
      │       ├── PostgreSQL Repositories
      │       ├── MongoDB Repository
      │       └── Redis Cache
      └── Middleware
          ├── Authentication
          ├── Tracing
          └── Error Handling
```

## Configuration

The service is configured via environment variables. See `.env.example` for available options.
