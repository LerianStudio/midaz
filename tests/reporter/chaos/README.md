# Chaos Tests

## Purpose
Validate system resilience and recovery under infrastructure failures.

## What We Test
- **RabbitMQ crashes** - Messages persist and are recovered
- **Worker crashes** - No message loss, graceful recovery
- **Combined failures** - Multiple components failing simultaneously
- **Message loss prevention** - Durable queues and persistent messages
- **Circuit breaker behavior** - Auto-recovery when datasources fail

## Key Guarantees
✅ **Zero message loss** - Messages survive RabbitMQ crashes  
✅ **Auto-recovery** - System recovers without manual intervention  
✅ **Status consistency** - Reports never stuck in "Processing"  
✅ **DLQ safety net** - Critical infrastructure failures captured  

## Running
```bash
ORG_ID=your-org-id make test-chaos
```

## Test Categories
- **DLQ Recovery** - Dead Letter Queue handling
- **RabbitMQ Resilience** - Message broker failures
- **Worker Resilience** - Consumer failures
- **Circuit Breaker** - Datasource protection

