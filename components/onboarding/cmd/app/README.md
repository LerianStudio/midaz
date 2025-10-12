# Onboarding Service Entrypoint

This is the main binary entrypoint for the Midaz onboarding service.

## Purpose

The onboarding service manages master data for the financial ledger:

- Organizations and their hierarchy
- Ledgers within organizations
- Portfolios and segments for grouping
- Account definitions and types
- Asset and currency configuration

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
- `REDIS_URL` - Cache connection
- `PORT` - HTTP server port (default: 3000)

See `.env.example` for full configuration options.

## API Documentation

Once running, the Swagger documentation is available at:

- http://localhost:3000/swagger/index.html

## Health Checks

The service exposes health check endpoints:

- `/health` - Overall service health
- `/readiness` - Readiness probe for Kubernetes

## Architecture

The service follows a layered architecture:

1. HTTP handlers receive requests
2. Command/Query use cases process business logic
3. Repositories handle data persistence
4. Infrastructure adapters integrate external services
