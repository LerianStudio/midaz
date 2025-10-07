# Package gold

## Overview

The `gold` package provides the Gold DSL (Domain-Specific Language) parser and validator for the Midaz ledger system. Gold DSL is a proprietary language for defining complex financial transactions with a concise, readable syntax.

## Purpose

This package enables:

- **Transaction definition**: Define complex n:n transactions using DSL syntax
- **Syntax validation**: Validate DSL syntax before execution
- **Parsing**: Convert DSL text to structured transaction objects
- **Amount distribution**: Support for fixed amounts, percentages, and remaining balances
- **Asset rate conversion**: Define exchange rates within transactions
- **Metadata support**: Attach custom data to transactions and operations

## Package Structure

```
gold/
├── Transaction.g4       # ANTLR4 grammar definition for Gold DSL
├── parser/              # ANTLR-generated parser code (auto-generated)
│   ├── transaction_lexer.go
│   ├── transaction_parser.go
│   ├── transaction_visitor.go
│   ├── transaction_listener.go
│   └── ... (other generated files)
└── transaction/         # Transaction parsing and validation logic
    ├── parse.go         # DSL parsing implementation
    ├── validate.go      # DSL syntax validation
    ├── error.go         # Error handling structures
    └── README.md        # This file
```

## Gold DSL Syntax

### Basic Structure

```lisp
(transaction V1
  (chart-of-accounts-group-name <UUID>)
  (description "<text>")
  (code <UUID>)
  (pending true|false)
  (metadata
    (key1 value1)
    (key2 value2))
  (send <ASSET> <amount> | <scale>
    (source
      (from <account> <send-type>))
    (distribute
      (to <account> <send-type>))))
```

### Components

#### Transaction Header

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "Payment settlement")
  (code 987f6543-a12b-34c5-d678-901234567890)
  (pending false)
  ...)
```

**Fields:**

- `chart-of-accounts-group-name`: Ledger ID (required)
- `description`: Human-readable description (optional)
- `code`: Transaction template code (optional)
- `pending`: Whether transaction is pending (optional, default: false)

#### Metadata

```lisp
(metadata
  (invoice_id 12345)
  (customer_ref ABC-001)
  (department sales))
```

**Format:**

- Key-value pairs in parentheses
- Keys and values can be UUIDs or integers
- Attached to transactions, from, or to operations

#### Send Specification

```lisp
(send USD 1000 | 1000
  (source ...)
  (distribute ...))
```

**Format:**

- Asset code (e.g., USD, BTC)
- Amount | Scale (e.g., 1000 | 1000 means $10.00 with 2 decimal places)
- Source accounts (where money comes from)
- Distribute accounts (where money goes to)

#### Source Accounts

```lisp
(source
  (from @customer_account :amount USD 1000 | 1000)
  (from @backup_account :remaining))
```

**Multiple sources supported:**

- Each `from` specifies a source account
- Can use fixed amounts, percentages, or remaining balance

#### Destination Accounts

```lisp
(distribute
  (to @revenue_account :share 60)
  (to @commission_account :share 40))
```

**Multiple destinations supported:**

- Each `to` specifies a destination account
- Can use fixed amounts, percentages, or remaining balance

### Send Types

#### Fixed Amount

```lisp
:amount USD 1000 | 1000
```

**Format:**

- `:amount <asset> <amount> | <scale>`
- Specifies exact amount to send/receive
- Amount and scale must match transaction send amount

#### Percentage Share

```lisp
:share 60
```

**Format:**

- `:share <percentage>`
- Percentage of the transaction amount (0-100)
- Multiple shares must sum to 100 or use :remaining

#### Percentage of Percentage

```lisp
:share 50 :of 60
```

**Format:**

- `:share <percentage> :of <percentage>`
- 50% of 60% = 30% of total
- Useful for cascading commission calculations

#### Remaining Balance

```lisp
:remaining
```

**Format:**

- `:remaining`
- Uses whatever amount is left after other operations
- Can be used at source, distribute, or individual from/to level

### Account References

Three ways to reference accounts:

```lisp
@customer_account           # Alias (starts with @)
123e4567-e89b-12d3-a456-426614174000  # UUID
$variable_name              # Variable (for templates)
```

### Asset Rate Conversion

```lisp
(from @usd_account :amount USD 100 | 100
  (rate ext-rate-id USD -> BTC 0.000025 | 1000000))
```

**Format:**

- `(rate <external-id> <from-asset> -> <to-asset> <rate> | <scale>)`
- Allows cross-asset transactions
- Rate is applied during transaction processing

### Complete Examples

#### Simple Transfer

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "Customer payment")
  (send USD 10000 | 100
    (source
      (from @customer_account :amount USD 10000 | 100))
    (distribute
      (to @revenue_account :amount USD 10000 | 100))))
```

#### Split Payment

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "Commission split")
  (send USD 10000 | 100
    (source
      (from @customer_account :amount USD 10000 | 100))
    (distribute
      (to @revenue_account :share 70)
      (to @commission_account :share 30))))
```

#### Multi-Source with Remaining

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "Multi-source payment")
  (send USD 10000 | 100
    (source
      (from @primary_account :amount USD 7000 | 100)
      (from @backup_account :remaining))
    (distribute
      (to @merchant_account :amount USD 10000 | 100))))
```

#### Cross-Asset Transaction

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "USD to BTC conversion")
  (send USD 1000 | 100
    (source
      (from @usd_account :amount USD 1000 | 100))
    (distribute
      (to @btc_account :amount BTC 25000000 | 100000000
        (rate rate-123 USD -> BTC 0.000025 | 1000000)))))
```

#### Pending Transaction

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "Future payment")
  (pending true)
  (send USD 5000 | 100
    (source
      (from @customer_account :amount USD 5000 | 100))
    (distribute
      (to @escrow_account :amount USD 5000 | 100))))
```

#### With Metadata

```lisp
(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (description "Invoice payment")
  (metadata
    (invoice_id 12345)
    (customer_ref ABC-001))
  (send USD 10000 | 100
    (source
      (from @customer_account :amount USD 10000 | 100
        (description "Payment for invoice 12345")
        (metadata
          (payment_method credit_card))))
    (distribute
      (to @revenue_account :amount USD 10000 | 100
        (description "Revenue from invoice 12345")))))
```

## API Reference

### Parse Function

```go
func Parse(dsl string) any
```

Parses Gold DSL into a transaction structure.

**Parameters:**

- `dsl`: Gold DSL string

**Returns:**

- `any`: `libTransaction.Transaction` struct (type assert required)

**Example:**

```go
import "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"

dslContent := `(transaction V1 ...)`
result := transaction.Parse(dslContent)
tx := result.(libTransaction.Transaction)
```

### Validate Function

```go
func Validate(dsl string) *Error
```

Validates Gold DSL syntax without parsing.

**Parameters:**

- `dsl`: Gold DSL string to validate

**Returns:**

- `*Error`: Error struct with syntax errors, or `nil` if valid

**Example:**

```go
if err := transaction.Validate(dslContent); err != nil {
    // Handle syntax errors
    for _, compileErr := range err.Errors {
        fmt.Printf("Line %d, Column %d: %s\n",
            compileErr.Line, compileErr.Column, compileErr.Message)
    }
    return constant.ErrInvalidScriptFormat
}
// DSL is valid, proceed with Parse()
```

### Error Types

#### Error

Collection of syntax errors:

```go
type Error struct {
    Errors []CompileError  // List of syntax errors
    Source string          // "lexer" or "parser"
}
```

#### CompileError

Individual syntax error:

```go
type CompileError struct {
    Line    int     // Line number (1-indexed)
    Column  int     // Column number (0-indexed)
    Message string  // Error message from ANTLR
    Source  string  // Error source
}
```

## Usage Patterns

### Validate Then Parse

```go
// 1. Validate syntax
if err := transaction.Validate(dslContent); err != nil {
    return fmt.Errorf("syntax error: %w", err)
}

// 2. Parse to struct
result := transaction.Parse(dslContent)
tx := result.(libTransaction.Transaction)

// 3. Process transaction
processedTx, err := service.CreateTransaction(tx)
```

### Handle Syntax Errors

```go
if err := transaction.Validate(dslContent); err != nil {
    var errorMessages []string
    for _, compileErr := range err.Errors {
        msg := fmt.Sprintf("Line %d, Column %d: %s",
            compileErr.Line, compileErr.Column, compileErr.Message)
        errorMessages = append(errorMessages, msg)
    }

    return pkg.ValidationError{
        Code:    constant.ErrInvalidScriptFormat.Error(),
        Title:   "Invalid DSL Syntax",
        Message: strings.Join(errorMessages, "; "),
    }
}
```

### Extract Transaction Details

```go
result := transaction.Parse(dslContent)
tx := result.(libTransaction.Transaction)

// Access transaction fields
ledgerID := tx.ChartOfAccountsGroupName
description := tx.Description
isPending := tx.Pending

// Access send details
asset := tx.Send.Asset
totalAmount := tx.Send.Value

// Access source accounts
for _, from := range tx.Send.Source.From {
    accountAlias := from.AccountAlias
    if from.Amount != nil {
        amount := from.Amount.Value
    }
    if from.Share != nil {
        percentage := from.Share.Percentage
    }
}

// Access destination accounts
for _, to := range tx.Send.Distribute.To {
    accountAlias := to.AccountAlias
    // Similar to source accounts
}
```

## DSL Grammar

The Gold DSL grammar is defined in `Transaction.g4` using ANTLR4 syntax.

### Tokens

- **VERSION**: `V1` (version identifier)
- **INT**: Integer numbers `[0-9]+`
- **STRING**: Quoted strings `"text"`
- **UUID**: Alphanumeric identifiers `[a-zA-Z0-9_\-/]+`
- **REMAINING**: `:remaining` keyword
- **VARIABLE**: Variables `$variable_name`
- **ACCOUNT**: Account aliases `@account_name`
- **WS**: Whitespace (skipped)

### Grammar Rules

See `Transaction.g4` for the complete grammar definition.

## Amount and Scale

Gold DSL uses a two-part format for amounts: `amount | scale`

**Examples:**

- `1000 | 100` = 10.00 (1000 cents, scale 100 = 2 decimal places)
- `1000 | 1000` = 1.000 (1000 units, scale 1000 = 3 decimal places)
- `100000000 | 100000000` = 1.00000000 (8 decimal places, for BTC)

**Why this format?**

- Avoids floating-point precision errors
- Supports arbitrary decimal precision
- Explicit about decimal places
- Compatible with shopspring/decimal

**Conversion:**

```go
// DSL: 1000 | 100
amount := decimal.NewFromInt(1000)
scale := decimal.NewFromInt(100)
actualValue := amount.Div(scale)  // 10.00
```

## Distribution Strategies

### Fixed Amounts

All amounts explicitly specified:

```lisp
(source
  (from @account1 :amount USD 500 | 100)
  (from @account2 :amount USD 500 | 100))
(distribute
  (to @account3 :amount USD 1000 | 100))
```

### Percentage Shares

Amounts calculated as percentages:

```lisp
(source
  (from @account1 :amount USD 1000 | 100))
(distribute
  (to @account2 :share 60)
  (to @account3 :share 40))
```

### Remaining Balance

One operation uses whatever is left:

```lisp
(source
  (from @account1 :amount USD 700 | 100)
  (from @account2 :remaining))
(distribute
  (to @account3 :amount USD 1000 | 100))
```

### Mixed Strategies

Combine different strategies:

```lisp
(source
  (from @account1 :amount USD 1000 | 100))
(distribute
  (to @account2 :share 50)
  (to @account3 :amount USD 300 | 100)
  (to @account4 :remaining))
```

## Asset Rate Conversion

Define exchange rates within transactions:

```lisp
(from @usd_account :amount USD 1000 | 100
  (rate rate-id-123 USD -> BTC 0.000025 | 1000000))
```

**Format:**

- `(rate <external-id> <from-asset> -> <to-asset> <rate> | <scale>)`
- External ID: Reference to asset rate record
- From/To assets: Asset codes being converted
- Rate and scale: Exchange rate value

**Use Cases:**

- Cross-currency transactions
- Cryptocurrency conversions
- Multi-asset operations

## Variables and Templates

Gold DSL supports variables for transaction templates:

```lisp
(transaction-template V1
  (chart-of-accounts-group-name $ledger_id)
  (description "Template transaction")
  (send $asset $amount | 100
    (source
      (from $source_account :amount $asset $amount | 100))
    (distribute
      (to $dest_account :amount $asset $amount | 100))))
```

**Variables:**

- Start with `$` (e.g., `$amount`, `$account`)
- Replaced with actual values when template is instantiated
- Used for reusable transaction patterns

## Validation

### Syntax Validation

```go
err := transaction.Validate(dslContent)
if err != nil {
    // Syntax errors found
    for _, compileErr := range err.Errors {
        fmt.Printf("Line %d, Column %d: %s\n",
            compileErr.Line, compileErr.Column, compileErr.Message)
    }
}
```

### Common Syntax Errors

**Missing parentheses:**

```lisp
transaction V1 ...  # Error: missing opening (
```

**Invalid token:**

```lisp
(transaction V2 ...)  # Error: V2 not recognized (should be V1)
```

**Mismatched amounts:**

```lisp
(send USD 1000 | 100 ...)  # Send amount
(from @account :amount USD 500 | 100)  # Error: doesn't match send amount
```

## Integration with Transaction Service

### HTTP Handler Pattern

```go
func createTransactionFromDSL(c *fiber.Ctx) error {
    // 1. Extract DSL file
    dslContent, err := http.GetFileFromHeader(c)
    if err != nil {
        return http.WithError(c, err)
    }

    // 2. Validate syntax
    if err := transaction.Validate(dslContent); err != nil {
        return http.BadRequest(c, pkg.ValidationError{
            Code:    constant.ErrInvalidScriptFormat.Error(),
            Title:   "Invalid DSL Syntax",
            Message: formatDSLErrors(err),
        })
    }

    // 3. Parse DSL
    result := transaction.Parse(dslContent)
    tx := result.(libTransaction.Transaction)

    // 4. Process transaction
    processedTx, err := service.CreateTransaction(tx)
    if err != nil {
        return http.WithError(c, err)
    }

    return http.Created(c, processedTx)
}
```

### Service Layer Pattern

```go
func (s *TransactionService) CreateFromDSL(dsl string) (*Transaction, error) {
    // Validate syntax
    if err := transaction.Validate(dsl); err != nil {
        return nil, constant.ErrInvalidScriptFormat
    }

    // Parse DSL
    result := transaction.Parse(dsl)
    tx := result.(libTransaction.Transaction)

    // Validate semantics
    if err := s.validateTransaction(tx); err != nil {
        return nil, err
    }

    // Execute transaction
    return s.executeTransaction(tx)
}
```

## ANTLR Parser

### Generated Code

The `parser/` directory contains ANTLR-generated code:

- **transaction_lexer.go**: Tokenizes DSL text
- **transaction_parser.go**: Parses tokens into parse tree
- **transaction_visitor.go**: Visitor interface for traversing parse tree
- **transaction_listener.go**: Listener interface for walking parse tree
- **transaction_base_visitor.go**: Base visitor implementation
- **transaction_base_listener.go**: Base listener implementation

### Regenerating Parser

If you modify `Transaction.g4`, regenerate the parser:

```bash
# Install ANTLR4
brew install antlr  # macOS
# or download from https://www.antlr.org/

# Generate parser
cd pkg/gold
antlr4 -Dlanguage=Go -visitor -listener Transaction.g4 -o parser/

# Move generated files
mv parser/*.go parser/
```

## Design Principles

1. **Lisp-like Syntax**: S-expressions for simplicity and unambiguity
2. **Explicit Amounts**: All amounts include scale for precision
3. **Flexible Distribution**: Support for amounts, shares, and remaining
4. **Metadata Support**: Extensible with custom key-value pairs
5. **Variable Support**: Templates for reusable transaction patterns
6. **Type Safety**: Strong typing via ANTLR grammar
7. **Error Recovery**: Collect all errors, don't stop at first

## Best Practices

### Always Validate First

```go
// Good
if err := transaction.Validate(dsl); err != nil {
    return err
}
tx := transaction.Parse(dsl)

// Bad - may panic on syntax errors
tx := transaction.Parse(dsl)  // Could panic if invalid
```

### Use Decimal for Amounts

```go
// Good
amount := decimal.NewFromInt(1000)

// Bad - floating point errors
amount := 10.00  // Precision issues
```

### Specify Scale Explicitly

```go
// Good - explicit scale
(send USD 1000 | 100)  // $10.00

// Bad - ambiguous
(send USD 1000)  // Is this $1000.00 or $10.00?
```

### Use Remaining Wisely

```go
// Good - one remaining per level
(source
  (from @account1 :amount USD 700 | 100)
  (from @account2 :remaining))

// Bad - multiple remaining
(source
  (from @account1 :remaining)
  (from @account2 :remaining))  // Ambiguous
```

## Performance Considerations

### Parsing Overhead

- ANTLR parsing has overhead (lexing + parsing + visiting)
- Cache parsed transactions when possible
- Consider using JSON API for high-frequency operations

### Memory Usage

- Parse trees can be large for complex transactions
- Parser instances are not reused (created per parse)
- Consider streaming for very large DSL files

## Security Considerations

### Input Validation

- Always validate DSL syntax before parsing
- Limit DSL file size to prevent DoS
- Validate account references exist
- Check permissions before execution

### Injection Prevention

- DSL grammar prevents most injection attacks
- Account aliases are validated separately
- Metadata values are validated for length

## Limitations

### Current Limitations

1. **No Comments**: DSL doesn't support comments
2. **No Arithmetic**: Can't compute amounts in DSL (use variables)
3. **No Conditionals**: No if/else logic in DSL
4. **Single Asset**: Each transaction uses one primary asset
5. **Static Rates**: Rates must be specified, can't fetch dynamically

### Workarounds

- **Comments**: Use description field
- **Arithmetic**: Calculate in application code, use variables
- **Conditionals**: Generate different DSL based on conditions
- **Multi-Asset**: Use rate conversions
- **Dynamic Rates**: Fetch rate, inject into DSL

## Error Handling

### Syntax Errors

```go
err := transaction.Validate(dsl)
if err != nil {
    fmt.Printf("Syntax errors in %s:\n", err.Source)
    for _, e := range err.Errors {
        fmt.Printf("  Line %d, Column %d: %s\n", e.Line, e.Column, e.Message)
    }
}
```

### Semantic Errors

Semantic validation happens in the transaction service:

- Account existence
- Sufficient balances
- Permission checks
- Rate validity

## Dependencies

This package depends on:

- `github.com/antlr4-go/antlr/v4`: ANTLR4 runtime for Go
- `github.com/shopspring/decimal`: Precise decimal arithmetic
- `github.com/LerianStudio/lib-commons/v2`: Shared transaction types

## Related Packages

- `pkg/mmodel`: Transaction models
- `components/transaction`: Transaction processing service
- `pkg/net/http`: DSL file upload handling

## Testing

The package includes comprehensive tests:

- `parse_test.go`: Parsing tests
- `parity_test.go`: DSL-to-struct parity tests
- `error_test.go`: Error handling tests

Run tests:

```bash
go test ./pkg/gold/transaction/...
```

## Future Enhancements

Potential additions:

- Comments support in DSL
- Arithmetic expressions
- Conditional logic
- Function definitions
- Import/include statements
- Type checking
- Semantic validation in parser

## References

- ANTLR4 Documentation: https://www.antlr.org/
- Lisp S-expressions: https://en.wikipedia.org/wiki/S-expression
- Gold DSL Examples: See `tests/fixtures/` directory

## Version History

This package follows semantic versioning as part of the Midaz v3 module.

Current DSL version: **V1**
