# Configuration Parameters

**Navigation:** [Home](../../) > [Reference Materials](../README.md) > Configuration Parameters

This document provides a comprehensive reference for environment variables and configuration parameters used in the Midaz platform.

## Configuration Structure

Midaz uses environment variables for configuration, loaded from `.env` files for each component. Default values are provided in `.env.example` files.

## Common Configuration Parameters

These parameters are common across multiple components.

### Environment Settings

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `ENV_NAME` | Environment name | `development` | `production`, `staging`, `development` |
| `VERSION` | Service version | `v2.0.0` | `v2.1.0` |

### Server Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `SERVER_PORT` | HTTP server port | Varies by service | `3000` (Onboarding), `3001` (Transaction) |
| `SERVER_ADDRESS` | Server address with port | `:${SERVER_PORT}` | `:3000` |

### Database Configuration (PostgreSQL)

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `DB_HOST` | Primary database hostname | `midaz-postgres-primary` | `localhost` |
| `DB_PORT` | Primary database port | `5701` | `5432` |
| `DB_USER` | Database username | `midaz` | `postgres` |
| `DB_PASSWORD` | Database password | `lerian` | `strong_password` |
| `DB_NAME` | Database name | Varies by service | `onboarding`, `transaction` |
| `DB_MAX_OPEN_CONNS` | Maximum open connections | `3000` | `1000` |
| `DB_MAX_IDLE_CONNS` | Maximum idle connections | `3000` | `1000` |

### Replica Database Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `DB_REPLICA_HOST` | Replica database hostname | `midaz-postgres-replica` | `localhost` |
| `DB_REPLICA_PORT` | Replica database port | `5702` | `5433` |
| `DB_REPLICA_USER` | Replica database username | `midaz` | `postgres_read` |
| `DB_REPLICA_PASSWORD` | Replica database password | `lerian` | `read_password` |
| `DB_REPLICA_NAME` | Replica database name | Varies by service | `onboarding`, `transaction` |

### MongoDB Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `MONGO_URI` | MongoDB connection URI scheme | `mongodb` | `mongodb+srv` |
| `MONGO_HOST` | MongoDB hostname | `midaz-mongodb` | `localhost` |
| `MONGO_PORT` | MongoDB port | `5703` | `27017` |
| `MONGO_USER` | MongoDB username | `midaz` | `mongo_user` |
| `MONGO_PASSWORD` | MongoDB password | `lerian` | `mongo_password` |
| `MONGO_NAME` | MongoDB database name | Varies by service | `onboarding`, `transaction` |
| `MONGO_MAX_POOL_SIZE` | Maximum connection pool size | `1000` | `500` |

### Redis Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `REDIS_HOST` | Redis hostname | `midaz-redis` | `localhost` |
| `REDIS_PORT` | Redis port | `5704` | `6379` |
| `REDIS_USER` | Redis username | `midaz` | `redis_user` |
| `REDIS_PASSWORD` | Redis password | `lerian` | `redis_password` |

### RabbitMQ Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `RABBITMQ_URI` | RabbitMQ connection URI scheme | `amqp` | `amqp` |
| `RABBITMQ_HOST` | RabbitMQ hostname | `midaz-rabbitmq` | `localhost` |
| `RABBITMQ_PORT_HOST` | RabbitMQ host port | `3003` | `5672` |
| `RABBITMQ_PORT_AMQP` | RabbitMQ AMQP port | `3004` | `5673` |
| `RABBITMQ_DEFAULT_USER` | RabbitMQ username | Varies by service | `rabbitmq_user` |
| `RABBITMQ_DEFAULT_PASS` | RabbitMQ password | `lerian` | `rabbitmq_password` |
| `RABBITMQ_NUMBERS_OF_WORKERS` | Number of worker processes | `5` | `10` |
| `RABBITMQ_NUMBERS_OF_PREFETCH` | Prefetch count for workers | `10` | `20` |

### Logging Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `LOG_LEVEL` | Logging level | `debug` | `info`, `warn`, `error` |

### OpenTelemetry Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `ENABLE_TELEMETRY` | Enable OpenTelemetry | `false` | `true` |
| `OTEL_RESOURCE_SERVICE_NAME` | Service name for telemetry | Varies by service | `onboarding`, `transaction` |
| `OTEL_RESOURCE_SERVICE_VERSION` | Service version for telemetry | `${VERSION}` | `v2.0.0` |
| `OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT` | Deployment environment | `${ENV_NAME}` | `production` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry collector endpoint | `midaz-otel-lgtm:4317` | `localhost:4317` |

### Swagger Documentation

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `SWAGGER_TITLE` | API title for Swagger | Varies by service | `Onboarding API` |
| `SWAGGER_DESCRIPTION` | API description | Varies by service | `Documentation for the Midaz Onboarding API` |
| `SWAGGER_VERSION` | API version for Swagger | `${VERSION}` | `v2.0.0` |
| `SWAGGER_HOST` | Swagger host | `${SERVER_ADDRESS}` | `:3000` |
| `SWAGGER_SCHEMES` | Swagger schemes | `http` | `https` |

### Pagination Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `MAX_PAGINATION_LIMIT` | Maximum items per page | `100` | `500` |
| `MAX_PAGINATION_MONTH_DATE_RANGE` | Maximum month range for date filters | Varies by service | `3` (Onboarding), `1` (Transaction) |

## Service-Specific Configuration

### Transaction Service

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `RABBITMQ_BALANCE_CREATE_QUEUE` | Queue for balance creation | `transaction.balance_create.queue` | `transaction.balance_create.queue.v2` |
| `RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE` | Exchange for BTO operations | `transaction.transaction_balance_operation.exchange` | `transaction.transaction_balance_operation.exchange.v2` |
| `RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY` | Routing key for BTO operations | `transaction.transaction_balance_operation.key` | `transaction.transaction_balance_operation.key.v2` |
| `RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE` | Queue for BTO operations | `transaction.transaction_balance_operation.queue` | `transaction.transaction_balance_operation.queue.v2` |
| `AUDIT_LOG_ENABLED` | Enable audit logging | `false` | `true` |
| `RABBITMQ_AUDIT_EXCHANGE` | Exchange for audit logs | `audit.append_log.exchange` | `audit.append_log.exchange.v2` |
| `RABBITMQ_AUDIT_KEY` | Routing key for audit logs | `audit.append_log.key` | `audit.append_log.key.v2` |

### Onboarding Service

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `RABBITMQ_EXCHANGE` | Exchange for outgoing messages | `transaction.balance_create.exchange` | `transaction.balance_create.exchange.v2` |
| `RABBITMQ_KEY` | Routing key for outgoing messages | `transaction.balance_create.key` | `transaction.balance_create.key.v2` |

## Authentication Configuration

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `PLUGIN_AUTH_ENABLED` | Enable authentication | `false` | `true` |
| `PLUGIN_AUTH_HOST` | Authentication service host | Empty | `https://auth.midaz.io` |

## Configuration Best Practices

1. **Environment Separation**: Use different configuration values for development, staging, and production
2. **Secrets Management**: Never commit sensitive configuration (passwords, keys) to version control
3. **Default Fallbacks**: Provide sensible defaults for non-critical configuration
4. **Documentation**: Keep this documentation updated when adding new configuration parameters
5. **Validation**: Validate configuration values at startup to fail fast on misconfigurations

## Environment Variables Hierarchy

Configuration values are loaded in the following order, with later sources overriding earlier ones:

1. Default values in code
2. `.env` file in the component directory
3. Environment variables set in the host environment
4. Command-line flags (where applicable)

## Example .env File

```bash
# TRANSACTION
ENV_NAME=development
VERSION=v2.0.0
SERVER_PORT=3001
SERVER_ADDRESS=:${SERVER_PORT}

# DB POSTGRESQL
DB_HOST=midaz-postgres-primary
DB_USER=midaz
DB_NAME=transaction
DB_PASSWORD=secure_password
DB_PORT=5701

# REDIS
REDIS_HOST=midaz-redis
REDIS_PORT=5704
REDIS_PASSWORD=secure_redis_password

# LOG LEVEL
LOG_LEVEL=debug

# Additional component-specific configuration...
```