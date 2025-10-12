# Transaction Service Entrypoint

This is the main binary entrypoint for the Midaz transaction service.

## Purpose

The transaction service handles all ledger activity:

- Transaction creation and processing
- Balance calculations and updates
- Operation routing and validation
- Asset rate conversions
- Async transaction processing

## Running the Service

### Development

```bash
# From the component directory
make run

# Or directly
go run cmd/app/main.go
```

### Production

```bash
# Build the binary
make build

# Run the compiled binary
./app
```

## Configuration

The service is configured via environment variables. Required variables:

- `DB_HOST`, `DB_PORT`, `DB_NAME` - PostgreSQL connection
- `MONGODB_URI` - MongoDB connection for metadata
- `RABBITMQ_URL` - Message broker connection
- `REDIS_URL` - Cache and Lua script execution
- `PORT` - HTTP server port (default: 3001)
- `RABBITMQ_TRANSACTION_ASYNC` - Enable async processing (default: true)

See `.env.example` for full configuration options.

## API Documentation

Once running, the Swagger documentation is available at:

- http://localhost:3001/swagger/index.html

## Health Checks

The service exposes health check endpoints:

- `/health` - Overall service health including RabbitMQ and Redis
- `/readiness` - Readiness probe for Kubernetes

## Processing Modes

### Synchronous Mode

Direct HTTP request/response for immediate transaction processing.

### Asynchronous Mode

High-throughput processing via RabbitMQ queues:

- Transactions queued for processing
- Workers consume and process in parallel
- Failed transactions backed up to Redis for retry

## Architecture

The service implements CQRS pattern with multiple consumers:

1. **HTTP Server** - Synchronous API requests
2. **RabbitMQ Consumer** - Async transaction processing
3. **Redis Consumer** - Retry queue for resilience

All consumers share the same business logic through command/query use cases.
