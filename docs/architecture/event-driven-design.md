# Event-Driven Design

**Navigation:** [Home](../../) > [Architecture](../) > Event-Driven Design

This document describes how Midaz implements event-driven architecture to achieve scalability, loose coupling, and resilience in its services.

## Overview

Event-driven architecture is a design paradigm in which the flow of the program is determined by events such as user actions, sensor outputs, or messages from other programs. In Midaz, events are used for asynchronous communication between services, enabling decoupled, scalable processing of financial transactions.

The event-driven approach in Midaz enables:
- Asynchronous processing of long-running operations
- Loose coupling between services
- Improved scalability and resilience
- Complex transaction lifecycle management

## Core Concepts

### 1. Message Broker

RabbitMQ serves as the message broker in Midaz, facilitating communication between services:

- **Exchanges**: Named entities that receive messages and route them to queues
- **Queues**: Message buffers that hold messages until they are processed
- **Bindings**: Rules that determine how messages are routed from exchanges to queues
- **Producers**: Services that send messages to exchanges
- **Consumers**: Services that receive and process messages from queues

### 2. Event Types

Midaz uses several types of events:

- **Entity Creation Events**: Notify services when entities like accounts are created
- **Transaction Events**: Trigger transaction processing and validation
- **Balance Update Events**: Notify services when account balances change
- **Audit Events**: Record transaction operations for audit purposes

### 3. Asynchronous Processing

Long-running operations are processed asynchronously to avoid blocking:

- **Two-Phase Processing**: Validation followed by execution
- **Worker Pools**: Process messages in parallel
- **Retries**: Automatically retry failed operations
- **Idempotent Operations**: Safe to retry without side effects

## Implementation in Midaz

### RabbitMQ Configuration

Midaz configures RabbitMQ with direct exchanges and durable queues:

```
┌───────────────────────────────────────┐
│            RabbitMQ Server            │
│                                       │
│  ┌─────────────────────────────────┐  │
│  │      Direct Exchanges           │  │
│  │                                 │  │
│  │  ┌─────────────────────────┐    │  │
│  │  │transaction.balance_create│    │  │
│  │  └─────────────────────────┘    │  │
│  │                                 │  │
│  │  ┌─────────────────────────────┐  │  │
│  │  │transaction.bto.execute      │  │  │
│  │  └─────────────────────────────┘  │  │
│  └─────────────────────────────────┘  │
│                    │                  │
│                    ▼                  │
│  ┌─────────────────────────────────┐  │
│  │           Queues                │  │
│  │                                 │  │
│  │  ┌─────────────────────────┐    │  │
│  │  │transaction.balance_create│    │  │
│  │  └─────────────────────────┘    │  │
│  │                                 │  │
│  │  ┌─────────────────────────────┐  │  │
│  │  │transaction.bto.execute      │  │  │
│  │  └─────────────────────────────┘  │  │
│  └─────────────────────────────────┘  │
└───────────────────────────────────────┘
```

Key configurations include:
- Durable queues to survive broker restarts
- Persistent messages to prevent data loss
- Prefetch counts for optimized throughput
- Connection pooling for efficiency

### Event Producers

Producers send events to the message broker:

```go
// Example: RabbitMQ producer implementation
func (p *ProducerRabbitMQ) SendBalanceCreateQueue(ctx context.Context, headerID string, queueData *mmodel.QueueData) error {
    // Create message payload
    payload, err := json.Marshal(queueData)
    if err != nil {
        return errors.FromError(err).WithMessage("error marshaling queue data")
    }
    
    // Publish message
    err = p.ch.PublishWithContext(
        ctx,
        "transaction.balance_create.exchange", // exchange
        "",                                    // routing key
        false,                                // mandatory
        false,                                // immediate
        amqp.Publishing{
            DeliveryMode: amqp.Persistent,
            ContentType:  "application/json",
            Body:         payload,
            Headers: amqp.Table{
                "X-Header-ID": headerID,
            },
        },
    )
    
    if err != nil {
        return errors.FromError(err).WithMessage("error publishing to RabbitMQ")
    }
    
    return nil
}
```

### Event Consumers

Consumers receive and process events:

```go
// Example: RabbitMQ consumer implementation
func (c *ConsumerRabbitMQ) Consume(ctx context.Context) error {
    // Set up worker pool
    workerCount := c.cfg.MaxWorkers
    workerPool := make(chan struct{}, workerCount)
    
    // Start consuming
    msgs, err := c.ch.Consume(
        "transaction.balance_create.queue", // queue
        "",                               // consumer
        false,                            // auto-ack
        false,                            // exclusive
        false,                            // no-local
        false,                            // no-wait
        nil,                              // args
    )
    if err != nil {
        return errors.FromError(err).WithMessage("error consuming from queue")
    }
    
    for msg := range msgs {
        // Limit concurrent workers
        workerPool <- struct{}{}
        
        // Process message in goroutine
        go func(msg amqp.Delivery) {
            defer func() { <-workerPool }()
            
            // Parse message
            var queueData mmodel.QueueData
            err := json.Unmarshal(msg.Body, &queueData)
            if err != nil {
                // Log error and reject message
                c.logger.Error("failed to unmarshal message", err)
                msg.Nack(false, false)
                return
            }
            
            // Process message
            err = c.handleFunc(ctx, queueData)
            if err != nil {
                // Log error and reject message
                c.logger.Error("failed to process message", err)
                msg.Nack(false, true) // requeue
                return
            }
            
            // Acknowledge message
            msg.Ack(false)
        }(msg)
    }
    
    return nil
}
```

### Event Schemas

Events in Midaz follow a standard structure:

```go
// Queue data structure
type QueueData struct {
    OrganizationID uuid.UUID           `json:"organization_id"`
    LedgerID       uuid.UUID           `json:"ledger_id"`
    AccountID      uuid.UUID           `json:"account_id,omitempty"`
    Payload        map[string]any      `json:"payload"`
}
```

The `Payload` field contains entity-specific data serialized as JSON.

### Key Event Flows

#### 1. Account Creation Flow

```
┌─────────────┐         ┌───────────┐         ┌─────────────────┐
│ Onboarding  │         │ RabbitMQ  │         │ Transaction     │
│ Service     │         │           │         │ Service         │
└──────┬──────┘         └─────┬─────┘         └────────┬────────┘
       │                      │                         │
       │ Create Account       │                         │
       ├──────────────────────┘                         │
       │                                                │
       │ Publish Account Created Event                  │
       ├─────────────────────────────────────────────────►
       │                                                │
       │                                                │ Process Event
       │                                                ├────────────┐
       │                                                │            │
       │                                                │ Create     │
       │                                                │ Balance    │
       │                                                ◄────────────┘
       │                                                │
       │                                                │ Store Balance
       │                                                ├────────────┐
       │                                                │            │
       │                                                │            │
       │                                                ◄────────────┘
       │                                                │
```

#### 2. Transaction Processing Flow

```
┌─────────────┐         ┌───────────┐         ┌─────────────────┐
│ Transaction │         │ RabbitMQ  │         │ Transaction     │
│ API         │         │           │         │ Worker          │
└──────┬──────┘         └─────┬─────┘         └────────┬────────┘
       │                      │                         │
       │ Create Transaction   │                         │
       ├────────────┐         │                         │
       │            │         │                         │
       │ Validate   │         │                         │
       │ Transaction│         │                         │
       ◄────────────┘         │                         │
       │                      │                         │
       │ Publish BTO Execute Event                      │
       ├─────────────────────────────────────────────────►
       │                      │                         │
       │ Return Accepted 202  │                         │ Process Event
       ◄────────────┐         │                         ├────────────┐
                    │         │                         │            │
                    │         │                         │ Update     │
                    │         │                         │ Balances   │
                    │         │                         │            │
                    │         │                         │ Create     │
                    │         │                         │ Operations │
                    │         │                         │            │
                    │         │                         │ Update     │
                    │         │                         │ Transaction│
                    │         │                         │ Status     │
                    │         │                         ◄────────────┘
```

### Error Handling

Midaz implements robust error handling in event processing:

- **Message Acknowledgment**: Messages are only acknowledged after successful processing
- **Message Nacking**: Failed messages are rejected and optionally requeued
- **Retry Logic**: Failed operations can be retried with backoff
- **Dead Letter Queues**: Persistently failed messages can be routed to dead letter queues for inspection
- **Correlation IDs**: Events carry correlation IDs for tracing through the system
- **Structured Logging**: Detailed error logging with context
- **OpenTelemetry**: Distributed tracing for error diagnosis

### Idempotency

Operations are designed to be idempotent to safely handle retries:

- **Idempotency Keys**: Unique IDs to identify duplicate requests
- **Version-based Concurrency**: Balance updates use optimistic concurrency control
- **Status Tracking**: Transaction status prevents duplicate processing

## Benefits in Midaz

The event-driven architecture provides Midaz with several benefits:

1. **Scalability**: Services can scale independently based on message load
2. **Resilience**: Services can continue functioning when others are down
3. **Loose Coupling**: Services communicate without direct dependencies
4. **Performance**: Asynchronous processing improves responsiveness
5. **Auditability**: Event-based processing provides a clear audit trail

## Next Steps

- [Hexagonal Architecture](./hexagonal-architecture.md) - How events fit into the hexagonal architecture
- [Component Integration](./component-integration.md) - How components interact using events
- [Transaction Lifecycle](./data-flow/transaction-lifecycle.md) - How transactions flow through the system