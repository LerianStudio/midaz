# Transaction Flow Diagram

This ASCII diagram illustrates the transaction processing flow in Midaz:

```
+-------------+   Request    +--------------------+  Validate   +-----------------+
|             |------------->|                    |------------>|                 |
|    Client   |              | Transaction API    |             | Input Validation|
|             |<-------------|                    |<------------|                 |
+-------------+   Response   +--------------------+  Result     +-----------------+
                                    |                                  |
                                    | Create                           |
                                    v                                  |
                     +--------------------+                            |
                     |                    |         Publish            |
                     | Postgres Database  |<----------------------+    |
                     |                    |         Events        |    |
                     +--------------------+                       |    |
                                    |                             |    |
                                    | Retrieve                    |    |
                                    v                             |    |
+-----------------+  Validate  +--------------------+  Queue     |    |
|                 |<-----------|                    |----------->|    |
| Balance Check   |            | Transaction Service|            |    |
|                 |----------->|                    |            |    |
+-----------------+  Result    +--------------------+            |    |
                                    |                            |    |
                                    | Process                    |    |
                                    v                            |    |
+-----------------+  Update    +--------------------+            |    |
|                 |<-----------|                    |            |    |
| Account Balances|            | Balance Management |            |    |
|                 |----------->|                    |<-----------+    |
+-----------------+  Result    +--------------------+                 |
                                    |                                 |
                                    | Update                          |
                                    v                                 |
                     +--------------------+                           |
                     |                    |                           |
                     | Update Transaction |                           |
                     | Status             |<--------------------------+
                     |                    |         Error Handling
                     +--------------------+
                                    |
                                    | Notify
                                    v
                     +--------------------+
                     |                    |
                     | Event Publication  |
                     | (RabbitMQ)         |
                     |                    |
                     +--------------------+
```

## Flow Description

1. **Request Initiation**: Client submits a transaction request to the Transaction API
2. **Validation**: Input data is validated (format, required fields, etc.)
3. **Transaction Creation**: A new transaction record is created in the database with status 'PENDING'
4. **Balance Validation**: The system checks if accounts have sufficient balances
5. **Operation Processing**: For each operation in the transaction:
   - Account balances are updated (debits and credits)
   - Balance records are updated with new amounts
6. **Status Update**: Transaction status is updated to 'COMPLETED' or 'FAILED'
7. **Event Publication**: Events are published to RabbitMQ for downstream processing

The system uses idempotency keys to prevent duplicate transaction processing and maintains transaction atomicity through database transactions.

## Related Documentation
- [Implementing Transactions](../tutorials/implementing-transactions.md)
- [Transaction Processing](../components/transaction/transaction-processing.md)