# Transaction Telemetry Framework Usage Guide

This guide demonstrates how to use the transaction telemetry framework to instrument your operations across different entities.

## Basic Concepts

The telemetry framework is designed around these core concepts:

1. **EntityTelemetry**: Central manager for creating operation instances
2. **EntityOperation**: Represents a specific operation on an entity (create, update, delete, etc.)
3. **Metrics Types**:
   - **Systemic Metrics**: Operation counts and system-level telemetry
   - **Business Metrics**: Business-related measurements like transaction amounts
   - **Duration Metrics**: How long operations take
   - **Error Metrics**: Tracking errors that occur

## Usage Examples

### Basic Usage Pattern

```go
// Create a telemetry operation
op := uc.Telemetry.NewTransactionOperation("create", transactionID)

// Add attributes for better filtering/querying
op.WithAttribute("organization_id", organizationID)

// Start tracing
ctx = op.StartTrace(ctx)

// Record basic count metric
op.RecordSystemicMetric(ctx)

// Do your business logic...
result, err := doSomething()

// If there's an error, record it
if err != nil {
    op.RecordError(ctx, "operation_failed", err)
    op.End(ctx, "failed")
    return nil, err
}

// Record business metrics if needed
op.RecordBusinessMetric(ctx, "amount", 1000.0)

// End the operation with success
op.End(ctx, "success")

return result, nil
```

### Transaction Creation Example

```go
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, t *model.Transaction) (*transaction.Transaction, error) {
    // Create operation telemetry
    transactionID := pkg.GenerateUUIDv7().String()
    op := uc.Telemetry.NewTransactionOperation("create", transactionID)
    
    // Add important attributes
    op.WithAttributes(
        attribute.String("organization_id", organizationID.String()),
        attribute.String("ledger_id", ledgerID.String()),
        attribute.String("asset_code", t.Send.Asset),
    )
    
    // Start trace and record metric
    ctx = op.StartTrace(ctx)
    op.RecordSystemicMetric(ctx)
    
    // Business logic
    // ...
    
    // Record transaction amount as business metric
    op.RecordBusinessMetric(ctx, "amount", float64(t.Send.Value))
    
    // End operation
    op.End(ctx, "success")
    
    return result, nil
}
```

### Balance Update Example

```go
func (uc *UseCase) UpdateBalance(ctx context.Context, accountID string, assetCode string, amount float64) (*balance.Balance, error) {
    // Create balance operation
    op := uc.Telemetry.NewBalanceOperation("update", accountID)
    
    // Add attributes
    op.WithAttribute("asset_code", assetCode)
    
    // Start trace and record metric
    ctx = op.StartTrace(ctx)
    op.RecordSystemicMetric(ctx)
    
    // Attempt to update balance
    updatedBalance, err := uc.BalanceRepo.Update(ctx, accountID, assetCode, amount)
    if err != nil {
        op.RecordError(ctx, "balance_update_failed", err)
        op.End(ctx, "failed")
        return nil, err
    }
    
    // Record the balance update amount
    op.RecordBusinessMetric(ctx, "update", amount)
    
    // End operation
    op.End(ctx, "success")
    
    return updatedBalance, nil
}
```

### Batch Processing with Individual Operations

```go
func (uc *UseCase) CreateBalances(ctx context.Context, data mmodel.Queue) error {
    // Create a batch telemetry operation
    op := uc.Telemetry.NewBalanceOperation("create_batch", data.AccountID.String())
    
    // Add batch-level attributes
    op.WithAttributes(
        attribute.String("organization_id", data.OrganizationID.String()),
        attribute.Int("item_count", len(data.QueueData)),
    )
    
    // Start trace and record metric
    ctx = op.StartTrace(ctx)
    op.RecordSystemicMetric(ctx)
    
    successCount := 0
    errorCount := 0
    
    // Process each item in the batch
    for i, item := range data.QueueData {
        // Create an individual operation for each item
        itemOp := uc.Telemetry.NewBalanceOperation("create_item", item.ID.String())
        itemOp.WithAttribute("item_index", strconv.Itoa(i))
        
        // Start trace and record metric for this item
        itemCtx := itemOp.StartTrace(ctx)
        itemOp.RecordSystemicMetric(itemCtx)
        
        // Process the individual item
        err := processItem(itemCtx, item)
        if err != nil {
            // Record error for this item
            itemOp.RecordError(itemCtx, "processing_error", err)
            itemOp.End(itemCtx, "failed")
            errorCount++
        } else {
            // Record success for this item
            itemOp.End(itemCtx, "success")
            successCount++
        }
    }
    
    // Record batch results
    op.WithAttributes(
        attribute.Int("success_count", successCount),
        attribute.Int("error_count", errorCount),
    )
    
    // End batch operation
    if errorCount > 0 && successCount > 0 {
        op.End(ctx, "partial_success")
    } else if errorCount > 0 {
        op.End(ctx, "failed")
    } else {
        op.End(ctx, "success")
    }
    
    return nil
}
```

### Nested Operations with Helper Functions

```go
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string) ([]*operation.Operation, error) {
    // Create main operation
    op := uc.Telemetry.NewOperationOperation("create_batch", transactionID)
    
    // Start trace and record metric
    ctx = op.StartTrace(ctx)
    op.RecordSystemicMetric(ctx)
    
    var operations []*operation.Operation
    
    for _, balance := range balances {
        // Create individual operation for each balance
        balanceOp := uc.Telemetry.NewOperationOperation("create", balance.ID)
        balanceCtx := balanceOp.StartTrace(ctx)
        
        // Create the operation
        operation, err := createSingleOperation(balanceCtx, balance, transactionID)
        
        // Attempt to create metadata if applicable
        if err == nil && operation.Metadata != nil {
            // Pass the balanceOp to the helper function to record errors there
            metadataErr := uc.CreateMetadata(balanceCtx, operation, balanceOp)
            if metadataErr != nil {
                // Still consider partial success if operation was created
                balanceOp.End(balanceCtx, "partial_success")
            } else {
                balanceOp.End(balanceCtx, "success")
            }
        } else if err != nil {
            balanceOp.RecordError(balanceCtx, "creation_error", err)
            balanceOp.End(balanceCtx, "failed")
        } else {
            balanceOp.End(balanceCtx, "success")
        }
        
        if err == nil {
            operations = append(operations, operation)
        }
    }
    
    // Record result metrics and end main operation
    op.WithAttribute("operation_count", strconv.Itoa(len(operations)))
    op.End(ctx, "success")
    
    return operations, nil
}

// Helper function that accepts an operation telemetry entity
func (uc *UseCase) CreateMetadata(ctx context.Context, o *operation.Operation, op *EntityOperation) error {
    if o.Metadata == nil {
        return nil
    }
    
    // Create metadata
    meta := &mongodb.Metadata{
        EntityID: o.ID,
        EntityName: reflect.TypeOf(operation.Operation{}).Name(),
        Data: o.Metadata,
    }
    
    err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(operation.Operation{}).Name(), meta)
    if err != nil && op != nil {
        // Record the error in the provided operation
        op.RecordError(ctx, "metadata_creation_error", err)
    }
    
    return err
}
```

## Available Entity Operation Types

Each entity has a specific constructor for ease of use:

```go
// For transactions
op := telemetry.NewTransactionOperation("create", transactionID)

// For balances
op := telemetry.NewBalanceOperation("update", accountID)

// For generic operations
op := telemetry.NewOperationOperation("execute", operationID)

// For asset rates
op := telemetry.NewAssetRateOperation("fetch", assetCode)

// For custom entities
op := telemetry.NewEntityOperation("custom_entity", "process", entityID)
```

## Best Practices

1. **Always End Operations**: Make sure to call `op.End()` in all code paths
2. **Record Errors**: Use `op.RecordError()` for detailed error tracking
3. **Consistent Action Names**: Use consistent action names like "create", "update", "delete"
4. **Attribute Filtering**: Add meaningful attributes for later filtering
5. **Business Metrics**: Record business metrics for valuable insights
6. **Status Values**: Use consistent status values like "success", "failed", "partial_success"
7. **Batch Processing**: For batch operations, create both a parent operation and child operations
8. **Helper Functions**: Pass operation objects to helper functions to allow them to record errors

## Benefits of This Framework

1. **Standardized Instrumentation**: Consistent approach across all transaction components
2. **Simplified Tracing**: Automatic correlation of metrics and traces
3. **Business Insights**: Easy tracking of business-relevant metrics
4. **Performance Monitoring**: Built-in duration tracking
5. **Error Reporting**: Standardized error tracking
6. **Context Propagation**: Tracing context flows through the system
7. **Reduced Boilerplate**: Consistent pattern reduces code duplication 