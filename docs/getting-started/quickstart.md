# Quickstart

**Navigation:** [Home](../../) > [Getting Started](./README.md) > Quickstart

This guide will help you get started with Midaz quickly. It covers the basic workflow for setting up financial structures and creating transactions.

## Prerequisites

Before starting, ensure you have:

- Completed the [Installation](./installation.md) process
- The MDZ CLI installed and configured
- Basic understanding of financial accounting concepts

## Quick Setup

### 1. Login to Midaz

```bash
mdz login
```

This will authenticate you with the Midaz platform. Follow the prompts to complete the login process.

### 2. Create Your Financial Structure

Midaz uses a hierarchical structure for financial entities. Let's create a basic structure.

#### Create an Organization

```bash
mdz organization create --name "My Company" --code "MYCO" --description "My test organization"
```

Note the organization ID returned in the response. You'll need it for subsequent commands.

#### Create a Ledger

```bash
mdz ledger create --organization-id $ORG_ID --name "Main Ledger" --code "MAIN" --description "Main financial ledger"
```

Note the ledger ID returned in the response.

#### Create an Asset

```bash
mdz asset create --organization-id $ORG_ID --ledger-id $LEDGER_ID --code "USD" --name "US Dollar" --symbol "$" --decimals 2
```

#### Create Accounts

Create two accounts to test transactions:

```bash
# Create source account
mdz account create --organization-id $ORG_ID --ledger-id $LEDGER_ID --name "Source Account" --alias "@source" --type "checking"

# Create destination account
mdz account create --organization-id $ORG_ID --ledger-id $LEDGER_ID --name "Destination Account" --alias "@destination" --type "savings"
```

### 3. Create Your First Transaction

Now that you have accounts set up, you can create a transaction between them.

#### Using the Transaction DSL

Create a file named `transaction.dsl` with the following content:

```
transaction "First Transfer" {
  description "Transfer from source to destination"
  code "TRANSFER"
  
  send USD 100.00 {
    source {
      from "@source" {
        chart_of_accounts "1000"
        description "Withdrawal from source account"
      }
    }
    
    distribute {
      to "@destination" {
        chart_of_accounts "2000"
        description "Deposit to destination account"
      }
    }
  }
}
```

Then execute the transaction:

```bash
curl -X POST "http://localhost:3001/v1/organizations/$ORG_ID/ledgers/$LEDGER_ID/transactions/dsl" \
  -H "Content-Type: multipart/form-data" \
  -F "transaction=@transaction.dsl"
```

### 4. Check Account Balances

You can now check the balances of your accounts:

```bash
# Check source account balance
curl "http://localhost:3001/v1/organizations/$ORG_ID/ledgers/$LEDGER_ID/accounts/@source/balances"

# Check destination account balance
curl "http://localhost:3001/v1/organizations/$ORG_ID/ledgers/$LEDGER_ID/accounts/@destination/balances"
```

The source account should show -100.00 USD, and the destination account should show +100.00 USD.

## Next Steps

Congratulations! You've completed the basic workflow for Midaz. Here are some next steps to explore:

- Learn about more complex [transaction patterns](../tutorials/implementing-transactions.md)
- Understand the [entity hierarchy](../domain-models/entity-hierarchy.md) in depth
- Explore [API integration](../tutorials/api-integration-examples.md)
- Set up [metadata fields](../domain-models/metadata-approach.md) for custom attributes

For more detailed documentation, refer to:

- [API Reference](../api-reference/README.md) for all available endpoints
- [Architecture Overview](./architecture-overview.md) for understanding the system design
- [Tutorials](../tutorials/README.md) for step-by-step guides