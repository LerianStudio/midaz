# Transaction DSL (Gold Language) Guide

## Overview

Midaz implements a custom **Domain-Specific Language (DSL)** called **Gold** for defining complex financial transactions. The DSL is parsed using **ANTLR4** (ANother Tool for Language Recognition), providing type-safe, validated transaction definitions.

**Location**: `pkg/gold/`

## Why a Transaction DSL?

Financial transactions in Midaz can be complex:
- **n:n transactions**: Multiple sources to multiple destinations
- **Distribution rules**: Percentage-based, ratio-based, or remaining amounts
- **Transaction metadata**: Additional context and tracking
- **Validation**: Ensure transaction integrity before execution

The Gold DSL provides a human-readable, type-safe way to define these complex scenarios.

## Grammar Definition

**Location**: `pkg/gold/Transaction.g4`

The ANTLR4 grammar defines the transaction language structure:

```antlr
grammar Transaction;

// Top-level transaction definition
transaction
    : 'send' amount 'from' source 'to' destination metadata?
    | 'distribute' amount 'from' source 'to' destinations distribution metadata?
    ;

// Amount can be a number or a variable
amount
    : NUMBER
    | '@' IDENTIFIER  // Variable reference
    ;

// Source account
source
    : '@' IDENTIFIER
    ;

// Single destination
destination
    : '@' IDENTIFIER
    ;

// Multiple destinations
destinations
    : destination (',' destination)+
    ;

// Distribution rules
distribution
    : 'by' distributionRule (',' distributionRule)*
    ;

distributionRule
    : PERCENTAGE     // e.g., 50%
    | 'ratio' ratio   // e.g., ratio 1:2
    | 'remaining'     // Allocate remaining amount
    | 'rate' NUMBER   // Exchange rate
    ;

ratio
    : NUMBER ':' NUMBER
    ;

// Metadata
metadata
    : '{' metadataEntry (',' metadataEntry)* '}'
    ;

metadataEntry
    : STRING ':' (STRING | NUMBER | BOOLEAN)
    ;

// Lexer rules
PERCENTAGE : [0-9]+ '%' ;
NUMBER     : [0-9]+ ('.' [0-9]+)? ;
STRING     : '"' (~["\r\n])* '"' ;
BOOLEAN    : 'true' | 'false' ;
IDENTIFIER : [a-zA-Z_][a-zA-Z0-9_]* ;
WS         : [ \t\r\n]+ -> skip ;
```

## Transaction Examples

### Simple Transfer

```
send 100 from @checking to @savings
```

Transfers $100 from checking account to savings account.

### Transfer with Metadata

```
send 500 from @merchant to @supplier {
    "invoice": "INV-2025-001",
    "category": "supplies",
    "approved_by": "manager@example.com"
}
```

### Percentage-Based Distribution

```
distribute 1000 from @revenue to @account1, @account2, @account3 by 50%, 30%, 20%
```

Distributes $1000:
- @account1 receives $500 (50%)
- @account2 receives $300 (30%)
- @account3 receives $200 (20%)

### Ratio-Based Distribution

```
distribute 3000 from @investment to @partner1, @partner2, @partner3 by ratio 2:2:1
```

Distributes $3000 in 2:2:1 ratio:
- @partner1 receives $1200 (2/5)
- @partner2 receives $1200 (2/5)
- @partner3 receives $600 (1/5)

### Remaining Amount Distribution

```
distribute 5000 from @sales to @commission, @bonus, @pool by 10%, 5%, remaining
```

Distributes $5000:
- @commission receives $500 (10%)
- @bonus receives $250 (5%)
- @pool receives $4250 (remaining 85%)

### Variable Amounts

```
send @invoice_amount from @customer to @vendor {
    "invoice_id": "@invoice_123"
}
```

Amount comes from a variable, allowing dynamic transaction amounts.

### Currency Conversion

```
send 100 from @usd_account to @eur_account rate 0.92 {
    "conversion": "USD_to_EUR",
    "rate_date": "2025-12-14"
}
```

Converts $100 USD to €92 EUR using specified rate.

## Parsing Implementation

### Parser Setup

**Location**: `pkg/gold/parser.go`

```go
package gold

import (
    "github.com/antlr/antlr4/runtime/Go/antlr"
)

type TransactionParser struct {
    parser *Parser  // Generated ANTLR parser
}

func NewTransactionParser() *TransactionParser {
    return &TransactionParser{}
}

func (p *TransactionParser) Parse(input string) (*Transaction, error) {
    // Create input stream from string
    inputStream := antlr.NewInputStream(input)

    // Create lexer
    lexer := NewTransactionLexer(inputStream)

    // Create token stream
    tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)

    // Create parser
    parser := NewTransactionParser(tokens)

    // Add error listener
    errorListener := &TransactionErrorListener{}
    parser.RemoveErrorListeners()
    parser.AddErrorListener(errorListener)

    // Parse transaction
    tree := parser.Transaction()

    // Check for syntax errors
    if len(errorListener.errors) > 0 {
        return nil, fmt.Errorf("parse errors: %v", errorListener.errors)
    }

    // Convert parse tree to domain model
    visitor := NewTransactionVisitor()
    result := visitor.Visit(tree)

    transaction, ok := result.(*Transaction)
    if !ok {
        return nil, fmt.Errorf("failed to build transaction model")
    }

    return transaction, nil
}
```

### AST Visitor

```go
type TransactionVisitor struct {
    BaseTransactionVisitor
}

func (v *TransactionVisitor) VisitTransaction(ctx *TransactionContext) interface{} {
    // Extract transaction type
    if ctx.GetChild(0).GetText() == "send" {
        return v.visitSimpleTransfer(ctx)
    } else if ctx.GetChild(0).GetText() == "distribute" {
        return v.visitDistribution(ctx)
    }

    return nil
}

func (v *TransactionVisitor) visitSimpleTransfer(ctx *TransactionContext) *Transaction {
    return &Transaction{
        Type:        TransactionTypeSimple,
        Amount:      v.visitAmount(ctx.amount()),
        Source:      v.visitSource(ctx.source()),
        Destination: v.visitDestination(ctx.destination()),
        Metadata:    v.visitMetadata(ctx.metadata()),
    }
}

func (v *TransactionVisitor) visitDistribution(ctx *TransactionContext) *Transaction {
    return &Transaction{
        Type:         TransactionTypeDistribution,
        Amount:       v.visitAmount(ctx.amount()),
        Source:       v.visitSource(ctx.source()),
        Destinations: v.visitDestinations(ctx.destinations()),
        Distribution: v.visitDistribution(ctx.distribution()),
        Metadata:     v.visitMetadata(ctx.metadata()),
    }
}
```

### Domain Model

```go
type Transaction struct {
    Type         TransactionType
    Amount       Amount
    Source       AccountRef
    Destination  AccountRef        // For simple transfers
    Destinations []AccountRef      // For distributions
    Distribution DistributionRules // How to split amount
    Metadata     map[string]interface{}
}

type TransactionType string

const (
    TransactionTypeSimple       TransactionType = "simple"
    TransactionTypeDistribution TransactionType = "distribution"
)

type Amount struct {
    Value    decimal.Decimal
    Variable string  // If amount is @variable
}

type AccountRef struct {
    Identifier string  // e.g., "checking", "savings"
}

type DistributionRules struct {
    Rules []DistributionRule
}

type DistributionRule struct {
    Type       DistributionType
    Percentage decimal.Decimal  // For percentage
    Ratio      []int            // For ratio
    Rate       decimal.Decimal  // For exchange rate
}

type DistributionType string

const (
    DistributionTypePercentage DistributionType = "percentage"
    DistributionTypeRatio      DistributionType = "ratio"
    DistributionTypeRemaining  DistributionType = "remaining"
    DistributionTypeRate       DistributionType = "rate"
)
```

## Transaction Validation

### Semantic Validation

After parsing, validate transaction semantics:

```go
func (t *Transaction) Validate() error {
    // Validate amount
    if t.Amount.Value.IsZero() && t.Amount.Variable == "" {
        return errors.New("transaction amount cannot be zero")
    }

    if t.Amount.Value.IsNegative() {
        return errors.New("transaction amount cannot be negative")
    }

    // Validate source
    if t.Source.Identifier == "" {
        return errors.New("transaction source is required")
    }

    // Validate destinations
    if t.Type == TransactionTypeSimple {
        if t.Destination.Identifier == "" {
            return errors.New("transaction destination is required")
        }
    } else if t.Type == TransactionTypeDistribution {
        if len(t.Destinations) == 0 {
            return errors.New("distribution requires at least one destination")
        }

        // Validate distribution rules match destinations
        if len(t.Distribution.Rules) != len(t.Destinations) {
            return errors.New("distribution rules must match number of destinations")
        }

        // Validate percentage sum
        if err := t.validatePercentageSum(); err != nil {
            return err
        }
    }

    return nil
}

func (t *Transaction) validatePercentageSum() error {
    totalPercentage := decimal.Zero
    hasRemaining := false

    for _, rule := range t.Distribution.Rules {
        if rule.Type == DistributionTypePercentage {
            totalPercentage = totalPercentage.Add(rule.Percentage)
        } else if rule.Type == DistributionTypeRemaining {
            hasRemaining = true
        }
    }

    if !hasRemaining && !totalPercentage.Equal(decimal.NewFromInt(100)) {
        return fmt.Errorf("percentage sum must equal 100%%, got %s", totalPercentage)
    }

    if hasRemaining && totalPercentage.GreaterThanOrEqual(decimal.NewFromInt(100)) {
        return errors.New("percentages cannot sum to 100% with 'remaining' rule")
    }

    return nil
}
```

## Transaction Execution

### Executing Parsed Transaction

```go
func (s *TransactionService) Execute(ctx context.Context, dsl string) error {
    // 1. Parse DSL
    parser := gold.NewTransactionParser()
    tx, err := parser.Parse(dsl)
    if err != nil {
        return fmt.Errorf("parsing transaction DSL: %w", err)
    }

    // 2. Validate
    if err := tx.Validate(); err != nil {
        return fmt.Errorf("validating transaction: %w", err)
    }

    // 3. Resolve accounts
    sourceAccount, err := s.accountRepo.FindByIdentifier(ctx, tx.Source.Identifier)
    if err != nil {
        return fmt.Errorf("resolving source account: %w", err)
    }

    // 4. Check balance
    if sourceAccount.Balance.LessThan(tx.Amount.Value) {
        return pkg.ValidationError{
            Code:    constant.ErrInsufficientBalance,
            Message: "Insufficient balance for transaction",
        }
    }

    // 5. Execute based on type
    if tx.Type == gold.TransactionTypeSimple {
        return s.executeSimpleTransfer(ctx, tx, sourceAccount)
    } else {
        return s.executeDistribution(ctx, tx, sourceAccount)
    }
}

func (s *TransactionService) executeDistribution(ctx context.Context, tx *gold.Transaction, source *Account) error {
    // Calculate distributions
    distributions := s.calculateDistributions(tx.Amount.Value, tx.Distribution.Rules)

    // Start database transaction
    dbTx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("starting transaction: %w", err)
    }
    defer dbTx.Rollback()

    // Debit source
    if err := s.debitAccount(ctx, dbTx, source.ID, tx.Amount.Value); err != nil {
        return fmt.Errorf("debiting source: %w", err)
    }

    // Credit each destination
    for i, dest := range tx.Destinations {
        destAccount, err := s.accountRepo.FindByIdentifier(ctx, dest.Identifier)
        if err != nil {
            return fmt.Errorf("resolving destination %s: %w", dest.Identifier, err)
        }

        amount := distributions[i]
        if err := s.creditAccount(ctx, dbTx, destAccount.ID, amount); err != nil {
            return fmt.Errorf("crediting destination: %w", err)
        }
    }

    // Commit transaction
    if err := dbTx.Commit(); err != nil {
        return fmt.Errorf("committing transaction: %w", err)
    }

    return nil
}
```

## Generating ANTLR Parser

### Build Process

```bash
# Install ANTLR4
go install github.com/antlr/antlr4/runtime/Go/antlr@latest

# Generate parser from grammar
cd pkg/gold
antlr4 -Dlanguage=Go -o generated/ Transaction.g4

# Generated files:
# - TransactionLexer.go
# - TransactionParser.go
# - TransactionVisitor.go
# - TransactionBaseVisitor.go
```

### Integration in Build

Add to Makefile:
```makefile
generate-parser:
	cd pkg/gold && antlr4 -Dlanguage=Go -o generated/ Transaction.g4

build: generate-parser
	go build ./...
```

## Testing Transaction DSL

### Parser Tests

```go
func TestParse_SimpleTransfer(t *testing.T) {
    parser := gold.NewTransactionParser()

    input := `send 100 from @checking to @savings`

    tx, err := parser.Parse(input)

    require.NoError(t, err)
    assert.Equal(t, gold.TransactionTypeSimple, tx.Type)
    assert.Equal(t, decimal.NewFromInt(100), tx.Amount.Value)
    assert.Equal(t, "checking", tx.Source.Identifier)
    assert.Equal(t, "savings", tx.Destination.Identifier)
}

func TestParse_DistributionWithPercentages(t *testing.T) {
    parser := gold.NewTransactionParser()

    input := `distribute 1000 from @revenue to @account1, @account2, @account3 by 50%, 30%, 20%`

    tx, err := parser.Parse(input)

    require.NoError(t, err)
    assert.Equal(t, gold.TransactionTypeDistribution, tx.Type)
    assert.Equal(t, 3, len(tx.Destinations))
    assert.Equal(t, 3, len(tx.Distribution.Rules))
}

func TestParse_InvalidSyntax(t *testing.T) {
    parser := gold.NewTransactionParser()

    input := `send invalid syntax here`

    _, err := parser.Parse(input)

    assert.Error(t, err)
}
```

### Validation Tests

```go
func TestValidate_PercentageSum(t *testing.T) {
    tx := &gold.Transaction{
        Type:   gold.TransactionTypeDistribution,
        Amount: gold.Amount{Value: decimal.NewFromInt(1000)},
        Distribution: gold.DistributionRules{
            Rules: []gold.DistributionRule{
                {Type: gold.DistributionTypePercentage, Percentage: decimal.NewFromInt(60)},
                {Type: gold.DistributionTypePercentage, Percentage: decimal.NewFromInt(30)},
                // Missing 10% - should fail
            },
        },
    }

    err := tx.Validate()

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "percentage sum must equal 100%")
}
```

## Transaction DSL Checklist

✅ **Define clear grammar** in ANTLR4 format

✅ **Generate parser** from grammar file

✅ **Implement visitor** to convert parse tree to domain model

✅ **Validate semantics** after parsing

✅ **Handle parse errors** gracefully with clear messages

✅ **Test parser** with valid and invalid inputs

✅ **Document DSL syntax** for users

✅ **Provide examples** of common transaction patterns

## Related Documentation

- Architecture: `docs/agents/architecture.md`
- Error Handling: `docs/agents/error-handling.md`
- Testing: `docs/agents/testing.md`
- Database: `docs/agents/database.md`
