# RabbitMQ Configuration

**Navigation:** [Home](../../) > [Components](../) > [Infrastructure](./) > RabbitMQ

This document describes the RabbitMQ configuration and usage in the Midaz system.

## Overview

RabbitMQ serves as the message broker in the Midaz system, facilitating asynchronous communication between microservices. It follows an event-driven architecture pattern, allowing services to communicate without tight coupling.

In Midaz, RabbitMQ is primarily used for:

1. **Asynchronous Processing**: Delegating resource-intensive operations to be processed in the background
2. **Service Integration**: Communication between the onboarding and transaction services
3. **Event Distribution**: Propagating events like account creation to other services

## Messaging Architecture

Midaz employs a straightforward messaging architecture with RabbitMQ:

- **Direct Exchanges**: Messages are routed to specific queues based on routing keys
- **Durable Queues**: Ensures messages survive broker restarts
- **Persistent Messages**: Messages are stored on disk for reliability
- **Worker Distribution**: Multiple concurrent workers process messages from each queue

### Message Flow Patterns

The primary message flows in the system include:

1. **Balance Creation Flow**:
   - Onboarding service publishes account information
   - Transaction service consumes the message to create associated balances
   - Ensures account and balance creation are coordinated across services

2. **Balance Transaction Operation Flow**:
   - Transaction service publishes transaction operations
   - Handled asynchronously by dedicated workers
   - Ensures complex transactions are processed consistently

## RabbitMQ Configuration

### Server Configuration

RabbitMQ is configured with custom server settings:

```
# Allow guest user access from any host
loopback_users.guest = false

# Auto-load exchanges, queues, etc. from definitions file
management.load_definitions = /etc/rabbitmq/definitions.json

# Custom ports
listeners.tcp.default = 3003
management.tcp.port = 3004
```

### Users and Permissions

The system defines three administrator users:

| User | Password | Permissions | Tags |
|------|----------|------------|------|
| midaz | (hashed) | configure.*, write.*, read.* | administrator |
| onboarding | (hashed) | configure.*, write.*, read.* | administrator |
| transaction | (hashed) | configure.*, write.*, read.* | administrator |

All users have full permissions on the default vhost.

### Exchanges

The following exchanges are pre-configured:

| Exchange Name | Type | Durable | Purpose |
|---------------|------|---------|---------|
| transaction.balance_create.exchange | direct | yes | Routes balance creation messages |
| transaction.transaction_balance_operation.exchange | direct | yes | Routes transaction operation messages |

### Queues

The system uses these durable queues:

| Queue Name | Durable | Purpose |
|------------|---------|---------|
| transaction.balance_create.queue | yes | Holds messages for balance creation |
| transaction.transaction_balance_operation.queue | yes | Holds messages for transaction operations |

### Bindings

Exchanges and queues are connected with these bindings:

| Exchange | Queue | Routing Key |
|----------|-------|------------|
| transaction.balance_create.exchange | transaction.balance_create.queue | transaction.balance_create.key |
| transaction.transaction_balance_operation.exchange | transaction.transaction_balance_operation.queue | transaction.transaction_balance_operation.key |

## Message Structure

Messages follow a common structure defined in the `Queue` model:

```go
type Queue struct {
    OrganizationID uuid.UUID   `json:"organizationId"`
    LedgerID       uuid.UUID   `json:"ledgerId"`
    AuditID        uuid.UUID   `json:"auditId"`
    AccountID      uuid.UUID   `json:"accountId"`
    QueueData      []QueueData `json:"queueData"`
}

type QueueData struct {
    ID    uuid.UUID       `json:"id"`
    Value json.RawMessage `json:"value"`
}
```

This structure allows for flexible payload data while maintaining consistent fields for tracking and identification.

## Producer Implementation

The system implements a consistent producer pattern across services:

1. **Connection Management**:
   - Centralized RabbitMQ connection handling
   - Connection established during service startup
   - Panics if connection fails (critical dependency)

2. **Message Publishing**:
   - JSON serialization of structured messages
   - Delivery mode set to persistent (amqp.Persistent)
   - Tracing headers included for distributed tracing
   - Content type set to application/json

Example producer code:

```go
func (prmq *ProducerRabbitMQRepository) ProducerDefault(ctx context.Context, exchange, key string, queueMessage mmodel.Queue) (*string, error) {
    // Tracing and logging setup
    
    message, err := json.Marshal(queueMessage)
    if err != nil {
        // Error handling
        return nil, err
    }

    err = prmq.conn.Channel.Publish(
        exchange,
        key,
        false,
        false,
        amqp.Publishing{
            ContentType:  "application/json",
            DeliveryMode: amqp.Persistent,
            Headers: amqp.Table{
                libConstants.HeaderID: libCommons.NewHeaderIDFromContext(ctx),
            },
            Body: message,
        })
    
    // Error handling and logging
    
    return nil, nil
}
```

## Consumer Implementation

The consumer pattern follows a worker-based approach:

1. **Consumer Registration**:
   - Each queue has a dedicated handler function
   - Handlers registered through a unified `ConsumerRoutes` mechanism
   - Multiple worker goroutines process each queue

2. **Message Processing**:
   - QoS (prefetch) settings control message distribution
   - Messages acknowledged only after successful processing
   - Failed messages are negatively acknowledged and requeued
   - Context enriched with tracing and logging information

3. **Worker Configuration**:
   - Default: 5 worker goroutines per queue
   - Prefetch count: 10 messages per worker (50 total by default)
   - Configurable through environment variables

Example consumer code:

```go
func (cr *ConsumerRoutes) RunConsumers() error {
    for queueName, handler := range cr.routes {
        // Set Quality of Service (prefetch)
        err := cr.conn.Channel.Qos(
            cr.NumbersOfPrefetch,
            0,
            false,
        )
        
        // Start consuming from queue
        messages, err := cr.conn.Channel.Consume(
            queueName,
            "",
            false,  // auto-ack disabled
            false,
            false,
            false,
            nil,
        )
        
        // Start worker goroutines
        for i := 0; i < cr.NumbersOfWorkers; i++ {
            go func(workerID int, queue string, handlerFunc QueueHandlerFunc) {
                for msg := range messages {
                    // Set up context with tracing
                    
                    // Process message
                    err := handlerFunc(ctx, msg.Body)
                    if err != nil {
                        // Negative acknowledge on error
                        _ = msg.Nack(false, true)
                        continue
                    }
                    
                    // Acknowledge successful processing
                    _ = msg.Ack(false)
                }
            }(i, queueName, handler)
        }
    }
    
    return nil
}
```

## Monitoring & Observability

RabbitMQ operations are instrumented with:

1. **Distributed Tracing**:
   - OpenTelemetry integration for message production and consumption
   - Trace headers propagated between services
   - Span creation for each message operation

2. **Structured Logging**:
   - Context-aware logging throughout the messaging lifecycle
   - Error details captured in both logs and traces
   - Standard logging patterns for message operations

3. **RabbitMQ Management Interface**:
   - Access available on port 3004
   - Provides real-time monitoring of queues, exchanges, and connections
   - Allows for manual inspection and intervention if needed

## Running Locally

To run RabbitMQ locally using Docker:

```bash
cd components/infra
docker-compose up -d rabbitmq
```

This starts a RabbitMQ instance with all the necessary configuration and definitions.

## Best Practices

1. **Use Durable Exchanges and Queues** for reliability
2. **Set Persistent Delivery Mode** for messages that must not be lost
3. **Implement Proper Error Handling** with negative acknowledgments
4. **Use Multiple Workers** for throughput and concurrency
5. **Include Tracing Headers** for observability across services
6. **Follow the Repository Pattern** for consistent messaging interfaces
7. **Configure Appropriate QoS** to manage resource utilization

## Common Issues and Troubleshooting

1. **Connection Failures**:
   - Check network connectivity to RabbitMQ
   - Verify credentials are correct
   - Ensure RabbitMQ service is running

2. **Message Processing Errors**:
   - Check consumer logs for processing errors
   - Verify message format matches expected structure
   - Monitor dead letter queues if configured

3. **Performance Issues**:
   - Adjust worker count and prefetch values
   - Monitor queue length and processing rates
   - Consider separate queues for different message priorities

## Related Documentation

- [Event-Driven Design](../../architecture/event-driven-design.md)
- [Component Integration](../../architecture/component-integration.md)
- [Transaction Lifecycle](../../architecture/data-flow/transaction-lifecycle.md)
