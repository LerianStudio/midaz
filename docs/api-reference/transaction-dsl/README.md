# Transaction DSL Guide

**Navigation:** [Home](../../) > [API Reference](../) > [Transaction DSL](./README.md)

This document provides a comprehensive guide for using the Transaction Domain-Specific Language (DSL) in Midaz, which offers a concise, readable way to define complex financial transactions.

## Introduction

The Transaction DSL is a specialized language designed to express financial transactions within the Midaz platform. It allows you to define:

- Transaction metadata (description, code, chart of accounts)
- Source accounts for funds
- Destination accounts for funds
- Amount distributions and transfers
- Currency/asset conversions
- Additional metadata for auditing and reporting

## Syntax Overview

A Transaction DSL document has the following general structure:

```
transaction "Name" {
  description "Description of the transaction"
  code "TRANSACTION_CODE"
  
  send ASSET_CODE VALUE.SCALE {
    source {
      from ACCOUNT_ID {
        chart_of_accounts "ACCOUNT_CODE"
        description "Description of the source"
      }
    }
    
    distribute {
      to ACCOUNT_ID {
        chart_of_accounts "ACCOUNT_CODE"
        description "Description of the destination"
      }
    }
  }
}
```

## Grammar Elements

The DSL grammar is defined in `pkg/gold/Transaction.g4` and includes the following main elements:

### Transaction Definition

```
transaction "Payment" {
  description "Payment for invoice #123"
  code "PAY_001"
  chart-of-accounts-group-name "PAYMENTS"
  pending false
  ...
}
```

- `description`: Human-readable description of the transaction
- `code`: Unique identifier for the transaction type
- `chart-of-accounts-group-name`: Accounting classification
- `pending`: Boolean indicating if the transaction should be held for review

### Send Block

The `send` block defines the asset, amount, and scale for the entire transaction:

```
send USD 1000.00 {
  ...
}
```

This indicates a transfer of 1000.00 USD.

### Source Block

The `source` block defines where funds are coming from:

```
source {
  from "@account1" {
    amount USD 500.00
    chart_of_accounts "CHECKING"
    description "Withdrawal from checking account"
  }
  from "@account2" {
    amount USD 500.00
    chart_of_accounts "SAVINGS"
    description "Withdrawal from savings account"
  }
}
```

### Distribute Block

The `distribute` block defines where funds are going:

```
distribute {
  to "@merchant" {
    amount USD 1000.00
    chart_of_accounts "MERCHANT_PAYMENT"
    description "Payment to merchant"
  }
}
```

## Amount Specification Methods

The DSL supports several ways to specify amounts:

### Fixed Amount

```
amount USD 500.00
```

### Percentage Share

```
share 50
```

This indicates 50% of the total transaction amount.

### Percentage of Percentage

```
share 50 of 80
```

This indicates 50% of 80% of the total transaction amount.

### Remaining Amount

```
remaining
```

This indicates all remaining funds after other distributions.

## Currency Conversion

For transactions involving currency conversion:

```
from "@account" {
  amount USD 1000.00
  rate "RATE_ID" USD -> BRL 5.00
}
```

This shows conversion from USD to BRL at a rate of 5.00.

## Metadata

Additional data can be attached to any transaction component:

```
metadata {
  ("reference" "INV-123")
  ("customer_id" "CUST-456")
}
```

## Template Variables

The DSL supports template variables for reusable transaction templates:

```
transaction-template "Payment" {
  ...
  from "$source_account" {
    amount $asset_code $amount
  }
  ...
}
```

Variables start with `$` and are replaced with actual values when the template is used.

## Complete Examples

### Simple Payment

```
transaction "Simple Payment" {
  description "Payment from Person A to Person B"
  code "PAYMENT"
  chart-of-accounts-group-name "TRANSFERS"
  
  send USD 100.00 {
    source {
      from "@personA" {
        chart_of_accounts "1000"
        description "Debit from Person A's account"
      }
    }
    
    distribute {
      to "@personB" {
        chart_of_accounts "2000"
        description "Credit to Person B's account"
      }
    }
  }
}
```

### Multi-source Payment with Currency Conversion

```
transaction "Multi-source Payment with Conversion" {
  description "Payment from multiple sources with currency conversion"
  code "MULTI_PAY_FX"
  chart-of-accounts-group-name "INTERNATIONAL"
  
  send USD 1000.00 {
    source {
      from "@account1" {
        amount USD 600.00
        chart_of_accounts "1001"
      }
      from "@account2" {
        amount USD 400.00
        chart_of_accounts "1002"
      }
    }
    
    distribute {
      to "@foreignMerchant" {
        amount USD 1000.00
        rate "RATE_ID" USD -> EUR 0.85
        chart_of_accounts "2001"
        description "Payment to foreign merchant in EUR"
      }
    }
  }
}
```

### Split Payment

```
transaction "Split Payment" {
  description "Split payment to multiple recipients"
  code "SPLIT_PAY"
  chart-of-accounts-group-name "DISTRIBUTIONS"
  
  send USD 1000.00 {
    source {
      from "@payer" {
        chart_of_accounts "1000"
      }
    }
    
    distribute {
      to "@recipient1" {
        share 50
        chart_of_accounts "2001"
        description "50% share"
      }
      to "@recipient2" {
        share 30
        chart_of_accounts "2002"
        description "30% share"
      }
      to "@recipient3" {
        remaining
        chart_of_accounts "2003"
        description "Remaining 20%"
      }
    }
  }
}
```

## Usage via API

To use the Transaction DSL via the API, submit a DSL file to the endpoint:

```
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl
```

Use `multipart/form-data` with a form field named `transaction` containing the DSL file.

The API will validate the DSL syntax, parse the file, and execute the transaction if valid.

## Best Practices

1. **Readability**: Format your DSL files with consistent indentation and comments for clarity
2. **Balance**: Ensure that source and distribution amounts balance correctly
3. **Validation**: Test your DSL files with the validation tools before submission
4. **Templates**: Use templates for recurring transaction patterns
5. **Versions**: Include version information in your transaction codes or metadata

## Error Handling

Common syntax errors include:
- Missing closing parentheses or braces
- Incorrect asset codes
- Invalid account references
- Unbalanced amounts (source ≠ distribution)

The API will return detailed error messages indicating the line and position of syntax errors in your DSL file.

## Validation

You can validate your DSL files programmatically using the provided validation tools:

```go
import "github.com/LerianStudio/midaz/pkg/gold/transaction"

errorListener := transaction.Validate(dslContent)
if errorListener != nil && len(errorListener.Errors) > 0 {
    // Handle validation errors
}
```

## References

- [Transaction API Documentation](../transaction-api/README.md)
- [Financial Modeling Guide](../../domain-models/financial-model.md)