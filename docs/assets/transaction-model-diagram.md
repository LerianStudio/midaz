# Transaction Model Diagram

This ASCII diagram illustrates the core structure of the transaction model in Midaz:

```
+----------------+       +-----------------+
| Transaction    |       | Balance         |
|----------------|       |-----------------|
| id             |       | id              |
| parent_id      |       | account_id      |
| idempotency_key|       | asset_code      |
| status         |       | amount          |
| metadata       +------>| available_amount|
| created_at     |       | blocked_amount  |
| updated_at     |       | created_at      |
|                |       | updated_at      |
+-------+--------+       +-----------------+
        |
        | 1:n
        v
+-------+--------+       +-----------------+
| Operation      |       | Account         |
|----------------|       |-----------------|
| id             |       | id              |
| transaction_id +------>| portfolio_id    |
| account_id     |       | alias           |
| asset_code     |       | status          |
| type           |       | metadata        |
| amount         |       | created_at      |
| status         |       | updated_at      |
| created_at     |       |                 |
| updated_at     |       |                 |
+----------------+       +-----------------+
```

## Model Description

- **Transaction**: The top-level entity that represents a financial transaction
  - Contains one or more operations
  - Has a unique idempotency key to prevent duplicate submissions
  - Can reference a parent transaction (for related transactions)

- **Operation**: Individual debit/credit entry within a transaction
  - Links to a specific account and asset
  - Represents a specific amount movement (debit or credit)
  - Multiple operations must balance within a transaction

- **Balance**: Represents the current state of an account for a specific asset
  - Tracks total, available, and blocked amounts
  - Updated as transactions are processed

- **Account**: Represents a financial account in the system
  - Belongs to a portfolio in the entity hierarchy
  - Has one or more balances (one per asset)

## Related Documentation
- [Implementing Transactions](../tutorials/implementing-transactions.md)
- [Transaction Domain Model](../components/transaction/domain-model.md)