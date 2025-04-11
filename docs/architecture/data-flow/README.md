# Data Flow

**Navigation:** [Home](../../../) > [Architecture](../) > Data Flow

This section documents how data flows through the Midaz system, including entity lifecycle and transaction processing.

## Overview

Data flow in Midaz follows specific patterns depending on the type of data being processed. The primary data flows are:

1. **Entity Lifecycle**: How financial entities (organizations, ledgers, accounts) flow through the system
2. **Transaction Lifecycle**: How financial transactions and their operations are processed

Both flows leverage event-driven architecture for asynchronous processing and service integration.

## Key Concepts

### Event-Driven Communication

Midaz uses events to communicate between services. When an entity is created, updated, or deleted, events are published to allow other services to react. For example:

- When an account is created in the Onboarding service, an event is published
- The Transaction service consumes this event and creates a balance for the account
- No direct service calls are required, enabling loose coupling

### Command Query Responsibility Segregation (CQRS)

Midaz implements CQRS to separate read and write operations:

- **Commands**: Change the state of the system (e.g., create a transaction)
- **Queries**: Return data without changing state (e.g., get account details)

This separation allows for optimized data models for each type of operation.

### Asynchronous Processing

Long-running operations are processed asynchronously:

- Operations are submitted via API with quick validation
- Processing continues in the background via queues
- Clients receive immediate acknowledgment
- Status updates track the progress of operations

## Data Flow Diagrams

### Entity Creation Flow

```
┌───────────┐         ┌───────────────┐          ┌───────────┐         ┌────────────────┐
│           │         │               │          │           │         │                │
│   User    │         │   MDZ CLI     │          │ Onboarding│         │  Transaction   │
│           │         │               │          │  Service  │         │  Service       │
└─────┬─────┘         └───────┬───────┘          └─────┬─────┘         └────────┬───────┘
      │                       │                        │                        │
      │ Create Account        │                        │                        │
      ├──────────────────────►│                        │                        │
      │                       │                        │                        │
      │                       │ POST /accounts         │                        │
      │                       ├───────────────────────►│                        │
      │                       │                        │                        │
      │                       │                        │ Store in Database      │
      │                       │                        ├──────────┐             │
      │                       │                        │          │             │
      │                       │                        ◄──────────┘             │
      │                       │                        │                        │
      │                       │                        │ Publish Event          │
      │                       │                        ├───────────────────────►│
      │                       │                        │                        │
      │                       │                        │                        │ Create Balance
      │                       │                        │                        ├──────────┐
      │                       │                        │                        │          │
      │                       │                        │                        ◄──────────┘
      │                       │ 201 Created            │                        │
      │                       ◄───────────────────────┤                        │
      │                       │                        │                        │
      │ Success               │                        │                        │
      ◄──────────────────────┤                        │                        │
      │                       │                        │                        │
```

### Transaction Processing Flow

```
┌───────────┐         ┌───────────────┐          ┌─────────────────┐         ┌────────────────┐
│           │         │               │          │                 │         │                │
│   User    │         │   MDZ CLI     │          │  Transaction    │         │  Queue         │
│           │         │               │          │  Service API    │         │  Consumer      │
└─────┬─────┘         └───────┬───────┘          └─────┬───────────┘         └────────┬───────┘
      │                       │                        │                              │
      │ Create Transaction    │                        │                              │
      ├──────────────────────►│                        │                              │
      │                       │                        │                              │
      │                       │ POST /transactions     │                              │
      │                       ├───────────────────────►│                              │
      │                       │                        │                              │
      │                       │                        │ Validate Transaction         │
      │                       │                        ├──────────┐                   │
      │                       │                        │          │                   │
      │                       │                        ◄──────────┘                   │
      │                       │                        │                              │
      │                       │                        │ Store in Database            │
      │                       │                        ├──────────┐                   │
      │                       │                        │          │                   │
      │                       │                        ◄──────────┘                   │
      │                       │                        │                              │
      │                       │                        │ Publish to Queue             │
      │                       │                        ├─────────────────────────────►│
      │                       │                        │                              │
      │                       │ 202 Accepted           │                              │ Process Transaction
      │                       ◄───────────────────────┤                              ├──────────┐
      │                       │                        │                              │          │
      │ Transaction Submitted │                        │                              │          │
      ◄──────────────────────┤                        │                              │          │
      │                       │                        │                              │          │
      │                       │                        │                              │ Update Balances
      │                       │                        │                              ├──────────┐
      │                       │                        │                              │          │
      │                       │                        │                              ◄──────────┘
      │                       │                        │                              │
      │                       │                        │ Update Transaction Status    │
      │                       │                        ◄─────────────────────────────┤
      │                       │                        │                              │
```

## Documents in This Section

- [Entity Lifecycle](./entity-lifecycle.md) - The lifecycle of entities from creation to deletion
- [Transaction Lifecycle](./transaction-lifecycle.md) - The lifecycle of transactions and their processing

## Next Steps

- [Hexagonal Architecture](../hexagonal-architecture.md) - The architectural pattern used by Midaz
- [Event-Driven Design](../event-driven-design.md) - How events are used for communication
- [Component Integration](../component-integration.md) - How components work together