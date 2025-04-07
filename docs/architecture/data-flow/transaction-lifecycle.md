# Transaction Lifecycle

**Navigation:** [Home](../../../) > [Architecture](../) > [Data Flow](./) > Transaction Lifecycle

This document describes the lifecycle of transactions in Midaz, from creation to completion, and the flow of transaction data through the system.

## Transaction Overview

Transactions in Midaz represent financial operations between accounts. They implement double-entry accounting principles, ensuring that the sum of debits equals the sum of credits. Each transaction consists of multiple operations that record individual account movements.

## Transaction Status Lifecycle

Transactions move through multiple statuses during their lifecycle:

```
                   ┌─────────┐
                   │ CREATED │
                   └────┬────┘
                        │
                        ▼
                   ┌─────────┐       ┌──────────┐
                   │APPROVED │------>│ CANCELED │
                   └────┬────┘       └──────────┘
                        │
                        ▼
                   ┌─────────┐       ┌──────────┐
                   │ PENDING │------>│ DECLINED │
                   └────┬────┘       └──────────┘
                        │
                        ▼
                   ┌─────────┐
                   │  SENT   │
                   └─────────┘
```

**Status Definitions:**
- **CREATED**: Initial state when a transaction is first created
- **APPROVED**: Transaction has passed validation 
- **PENDING**: Transaction is being processed asynchronously
- **SENT**: Transaction has been successfully completed
- **CANCELED**: Transaction was manually canceled
- **DECLINED**: Transaction failed during processing

## Transaction Processing Flow

The transaction processing follows these steps:

```
┌──────────────┐     ┌───────────────┐     ┌─────────────────┐     ┌──────────────┐     ┌────────────────┐
│  HTTP API    │     │ Command Layer │     │ DSL/Validation  │     │  Async Queue │     │ BTO Processor  │
│  Controller  │────►│ Create Trans. │────►│ Processing      │────►│  Publication │────►│ (Balance/Trans)│
└──────────────┘     └───────────────┘     └─────────────────┘     └──────────────┘     └───────┬────────┘
       ▲                     │                                                                  │
       │                     │                                                                  │
       │                     ▼                                                                  ▼
       │               ┌───────────────┐                                                ┌────────────────┐
       │               │   Validate    │                                                │ Balance Updates│
       │               │   Transaction │                                                │ & Operations   │
       │               └───────────────┘                                                └───────┬────────┘
       │                                                                                       │
       │                                                                                       ▼
       │                                                                                ┌────────────────┐
       │                                                                                │ Status Updates │
       └────────────────────────────────────────────────────────────────────────────────┤ & Event Publish│
                                                                                        └────────────────┘
```

### 1. Transaction Creation

The creation process begins with:

1. **API Request**: Client submits a transaction request via API
2. **DSL Parsing**: If using Domain Specific Language (DSL), the transaction script is parsed
3. **Transaction Model Creation**: A transaction object is created with initial status CREATED
4. **Initial Validation**: Business rules and syntax are validated
5. **Persistence**: Transaction is stored in the database
6. **Async Processing**: Transaction is submitted for asynchronous processing via queue

Example transaction creation flow:

```go
// Transaction creation handler
func (u *CreateTransactionUseCase) Execute(ctx context.Context, transaction *mmodel.Transaction) (*mmodel.Transaction, error) {
    // Validate the transaction
    err := u.validateTransaction(ctx, transaction)
    if err != nil {
        return nil, err
    }
    
    // Set initial values
    transaction.ID = uuid.New()
    transaction.Status = "CREATED"
    transaction.CreatedAt = time.Now()
    transaction.UpdatedAt = time.Now()
    
    // Store in repository
    result, err := u.repository.Create(ctx, transaction)
    if err != nil {
        return nil, err
    }
    
    // Store metadata if any
    if transaction.Metadata != nil {
        err = u.metadataRepo.Store(ctx, "transaction", result.ID, transaction.Metadata)
        if err != nil {
            return nil, err
        }
    }
    
    // Submit for async processing
    err = u.sendToProcessingQueue(ctx, result)
    if err != nil {
        // Log error but continue
        log.Warn(ctx, "Failed to send transaction to processing queue", err)
    }
    
    return result, nil
}
```

### 2. Transaction Validation

Transactions are validated at multiple levels:

1. **Structural Validation**:
   - Proper format and required fields
   - Valid transaction script syntax (for DSL-based transactions)
   - Valid account references

2. **Business Rule Validation**:
   - Double-entry balance (debits equal credits)
   - Valid account statuses
   - Valid asset codes
   - Sufficient funds in source accounts

3. **Permission Validation**:
   - Accounts allow sending (for debits)
   - Accounts allow receiving (for credits)
   - User has permission for the organizations involved

Example validation code:

```go
func (v *TransactionValidator) Validate(ctx context.Context, transaction *mmodel.Transaction) error {
    // Check required fields
    if transaction.Description == "" {
        return errors.NewValidationError("description is required")
    }
    
    // Parse and validate transaction script if present
    if transaction.Script != "" {
        ops, err := v.scriptParser.Parse(transaction.Script)
        if err != nil {
            return errors.NewValidationError(fmt.Sprintf("invalid transaction script: %s", err.Error()))
        }
        
        // Check balance of operations
        if !v.checkBalancedOperations(ops) {
            return errors.NewValidationError("transaction operations must be balanced (debits = credits)")
        }
        
        // Check accounts exist and have sufficient funds
        for _, op := range ops {
            if op.Type == "DEBIT" {
                // Check account exists
                account, err := v.accountRepo.Find(ctx, op.AccountID)
                if err != nil {
                    return err
                }
                
                // Check account allows sending
                if !account.AllowSending {
                    return errors.NewValidationError(fmt.Sprintf("account %s does not allow sending", op.AccountID))
                }
                
                // Check sufficient balance
                balance, err := v.balanceRepo.FindByAccountIDAndAssetCode(ctx, op.AccountID, op.AssetCode)
                if err != nil {
                    return err
                }
                
                available, _ := decimal.NewFromString(balance.Available)
                amount, _ := decimal.NewFromString(op.Amount)
                
                if available.LessThan(amount) {
                    return errors.NewValidationError(fmt.Sprintf("insufficient balance in account %s", op.AccountID))
                }
            } else if op.Type == "CREDIT" {
                // Check account exists
                account, err := v.accountRepo.Find(ctx, op.AccountID)
                if err != nil {
                    return err
                }
                
                // Check account allows receiving
                if !account.AllowReceiving {
                    return errors.NewValidationError(fmt.Sprintf("account %s does not allow receiving", op.AccountID))
                }
            }
        }
    }
    
    return nil
}
```

### 3. Asynchronous Processing (BTO Pattern)

Midaz uses an asynchronous Balance-Transaction-Operation (BTO) pattern to process transactions:

1. **Queue Message Creation**:
   - Transaction details are packaged in a queue message
   - Message includes operation details and balance references

2. **Queue Publication**:
   - Message is published to RabbitMQ
   - Transaction status is updated to APPROVED

3. **Queue Consumption**:
   - Worker consumes the message from the queue
   - Worker updates the transaction status to PENDING

4. **Balance Updates**:
   - Worker loads and locks balances for all involved accounts
   - Updates are performed with optimistic concurrency control
   - Balance versions are incremented with each update

5. **Operation Creation**:
   - Operations are created for each account movement
   - Each operation links to transaction and records the balance change

6. **Status Update**:
   - Transaction status is updated to SENT when complete
   - If errors occur, status is updated to DECLINED

Example BTO processing flow:

```go
// BTO Execute handler
func (u *SendBTOExecuteAsyncUseCase) Execute(ctx context.Context, transaction *mmodel.Transaction, operations []*mmodel.Operation, balances []*mmodel.Balance) error {
    // Create queue data
    queueData := &mmodel.QueueData{
        OrganizationID: transaction.OrganizationID,
        LedgerID:       transaction.LedgerID,
        Payload: map[string]interface{}{
            "transaction_id": transaction.ID.String(),
            "operations":     serializeOperations(operations),
            "balances":       serializeBalances(balances),
        },
    }
    
    // Generate header ID for tracing
    headerID := uuid.New().String()
    
    // Update transaction status to APPROVED
    transaction.Status = "APPROVED"
    transaction.UpdatedAt = time.Now()
    
    _, err := u.transactionRepo.Update(ctx, transaction.ID, transaction)
    if err != nil {
        return err
    }
    
    // Publish to queue
    err = u.rabbitMQRepo.SendBTOExecuteAsync(ctx, headerID, queueData)
    if err != nil {
        return err
    }
    
    return nil
}

// BTO consumer handler
func (h *BTOExecuteHandler) Handle(ctx context.Context, data *mmodel.QueueData) error {
    // Extract data from message
    transactionID, _ := uuid.Parse(data.Payload["transaction_id"].(string))
    operations := deserializeOperations(data.Payload["operations"])
    balances := deserializeBalances(data.Payload["balances"])
    
    // Update transaction status to PENDING
    transaction, err := h.transactionRepo.Find(ctx, transactionID)
    if err != nil {
        return err
    }
    
    transaction.Status = "PENDING"
    transaction.UpdatedAt = time.Now()
    
    _, err = h.transactionRepo.Update(ctx, transaction.ID, transaction)
    if err != nil {
        return err
    }
    
    // Process operations and update balances
    err = h.processOperations(ctx, transaction, operations, balances)
    if err != nil {
        // Update transaction status to DECLINED on error
        transaction.Status = "DECLINED"
        transaction.UpdatedAt = time.Now()
        
        _, updateErr := h.transactionRepo.Update(ctx, transaction.ID, transaction)
        if updateErr != nil {
            return errors.Wrap(err, updateErr.Error())
        }
        
        return err
    }
    
    // Update transaction status to SENT
    transaction.Status = "SENT"
    transaction.UpdatedAt = time.Now()
    
    _, err = h.transactionRepo.Update(ctx, transaction.ID, transaction)
    if err != nil {
        return err
    }
    
    // Publish audit event
    err = h.publishAuditEvent(ctx, transaction, operations)
    if err != nil {
        log.Warn(ctx, "Failed to publish audit event", err)
    }
    
    return nil
}
```

## Double-Entry Accounting Implementation

Midaz implements double-entry accounting principles:

1. **Balance Equation**: Assets = Liabilities + Equity
2. **Debit and Credit**: Every transaction has equal debits and credits
3. **Account Types**:
   - Debit increases Asset and Expense accounts
   - Credit increases Liability, Equity, and Revenue accounts

The system enforces these principles through:

```
┌──────────────────┐                        ┌──────────────────┐
│  Source Account  │                        │ Target Account   │
│                  │                        │                  │
│  Balance: $100   │                        │  Balance: $50    │
└────────┬─────────┘                        └────────┬─────────┘
         │                                           │
         │  Operation: DEBIT $30                     │  Operation: CREDIT $30
         │  (Decrease Balance)                       │  (Increase Balance)
         ▼                                           ▼
┌──────────────────┐                        ┌──────────────────┐
│  Source Account  │                        │ Target Account   │
│                  │                        │                  │
│  Balance: $70    │                        │  Balance: $80    │
└──────────────────┘                        └──────────────────┘
```

### Operation Types

Operations represent individual account movements within a transaction:

1. **DEBIT Operations**:
   - Created for source accounts (FROM)
   - Reduce available balance in source accounts

2. **CREDIT Operations**:
   - Created for destination accounts (TO)
   - Increase available balance in destination accounts

Every transaction ensures the sum of DEBIT operations equals the sum of CREDIT operations.

## Balance Management

Balance updates are a critical aspect of transaction processing:

1. **Balance Locking**:
   - Balances are locked for update during processing
   - Optimistic concurrency control using version fields

2. **Balance Fields**:
   - `available`: Funds available for use
   - `on_hold`: Funds reserved but not yet finalized
   - `version`: Optimistic locking version

3. **Balance Update Process**:
   - Load balance with lock
   - Check version matches expected version
   - Update available and on-hold amounts
   - Increment version
   - Save updated balance

Example balance update:

```go
func (r *PostgreSQLRepository) Update(ctx context.Context, id uuid.UUID, balance *mmodel.Balance) (*mmodel.Balance, error) {
    query := `
        UPDATE balances 
        SET available = $1, on_hold = $2, updated_at = $3, version = version + 1
        WHERE id = $4 AND version = $5 AND deleted_at IS NULL
        RETURNING id, account_id, asset_code, available, on_hold, scale, version, account_type, allow_sending, allow_receiving, created_at, updated_at
    `
    
    var result mmodel.Balance
    err := r.db.QueryRowContext(
        ctx,
        query,
        balance.Available,
        balance.OnHold,
        time.Now(),
        id,
        balance.Version,
    ).Scan(
        &result.ID,
        &result.AccountID,
        &result.AssetCode,
        &result.Available,
        &result.OnHold,
        &result.Scale,
        &result.Version,
        &result.AccountType,
        &result.AllowSending,
        &result.AllowReceiving,
        &result.CreatedAt,
        &result.UpdatedAt,
    )
    
    if err == sql.ErrNoRows {
        return nil, errors.NewConcurrencyError("balance was updated by another transaction")
    }
    
    if err != nil {
        return nil, errors.FromError(err).WithMessage("error updating balance")
    }
    
    return &result, nil
}
```

## Transaction DSL

Midaz supports a Domain Specific Language (DSL) for defining transactions:

```
TRANSACTION "Purchase payment"
FROM account:"12345" AMOUNT 100.00 USD
TO account:"67890" AMOUNT 100.00 USD;
```

The DSL is parsed using ANTLR-generated parsers that validate the syntax and extract operations:

1. **Lexer and Parser**:
   - Convert text into tokens and parse tree
   - Define grammar rules for valid transaction syntax

2. **Visitor Pattern**:
   - Traverse the parse tree
   - Extract source and destination accounts
   - Create DEBIT and CREDIT operations

3. **Validation**:
   - Ensure balanced operations
   - Check valid account references
   - Validate asset codes and amounts

## Audit and Traceability

Transactions provide a complete audit trail through:

1. **Operation Records**:
   - Each operation records the balance before and after the change
   - Links to transaction, account, and asset

2. **Transaction Status History**:
   - Status changes are timestamped
   - Full history of transaction lifecycle

3. **Idempotency**:
   - External IDs prevent duplicate processing
   - Idempotency keys ensure operations aren't applied twice

4. **Correlation IDs**:
   - Header IDs for tracing requests through the system
   - Link API requests to queue messages and database operations

## Transaction Lifecycle Benefits

The transaction lifecycle in Midaz provides several benefits:

1. **Data Integrity**: Double-entry accounting ensures balanced operations
2. **Auditability**: Complete history of all financial movements
3. **Resilience**: Asynchronous processing with retry capabilities
4. **Scalability**: Queue-based processing allows for high throughput
5. **Consistency**: Optimistic concurrency control prevents race conditions

## Next Steps

- [Entity Lifecycle](./entity-lifecycle.md) - How entities flow through the system
- [Component Integration](../component-integration.md) - How components interact
- [Financial Model](../../domain-models/financial-model.md) - Learn about the financial model