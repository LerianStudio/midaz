# Entity Hierarchy Diagram

This ASCII diagram illustrates the hierarchical relationship between entities in Midaz:

```
+----------------------+
|                      |
|     Organization     |
|                      |
+----------+-----------+
           |
           | 1:n
           v
+----------+-----------+
|                      |
|       Ledger         |
|                      |
+----------+-----------+
           |
           | 1:n
           v
+----------+-----------+     +--------------------+
|                      |     |                    |
|        Asset         +---->+    Asset Rate      |
|                      |     |                    |
+----------+-----------+     +--------------------+
           |
           | 1:n
           v
+----------+-----------+
|                      |
|       Segment        |
|                      |
+----------+-----------+
           |
           | 1:n
           v
+----------+-----------+
|                      |
|      Portfolio       |
|                      |
+----------+-----------+
           |
           | 1:n
           v
+----------+-----------+     +--------------------+
|                      |     |                    |
|       Account        +---->+     Balance        |
|                      |     |                    |
+----------+-----------+     +--------------------+
```

## Entity Descriptions

- **Organization**: Top-level entity representing a business or organizational unit
  - Contains multiple ledgers
  - Examples: Companies, divisions, departments

- **Ledger**: Accounting book for a specific purpose
  - Contains multiple assets
  - Examples: General ledger, accounts receivable

- **Asset**: Represents a type of value that can be tracked
  - Has associated exchange rates (Asset Rates)
  - Examples: Currencies, securities, commodities

- **Segment**: Categorization within a specific asset
  - Used to group portfolios
  - Examples: Business lines, product categories

- **Portfolio**: Collection of accounts with a common purpose
  - Contains multiple accounts
  - Examples: Investment portfolios, customer groups

- **Account**: Individual financial account
  - Has balances for different assets
  - Examples: Customer account, internal account

- **Balance**: Represents the amount of a specific asset in an account
  - Linked to an account and asset
  - Tracks total, available, and blocked amounts

## Related Documentation
- [Entity Hierarchy](../domain-models/entity-hierarchy.md)
- [Financial Model](../domain-models/financial-model.md)