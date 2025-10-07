# Package parser

## Overview

This package contains **ANTLR-generated code** for parsing the Gold DSL (Domain-Specific Language). The code in this directory is automatically generated from the `Transaction.g4` grammar file and should **not be manually edited**.

## Purpose

This package provides the low-level parsing infrastructure:

- **Lexer**: Tokenizes Gold DSL text into tokens
- **Parser**: Parses tokens into a parse tree according to grammar rules
- **Visitor**: Interface for traversing the parse tree
- **Listener**: Interface for walking the parse tree

## Generated Files

```
parser/
├── transaction_lexer.go           # Tokenizer (lexical analysis)
├── transaction_parser.go          # Parser (syntax analysis)
├── transaction_visitor.go         # Visitor interface
├── transaction_base_visitor.go    # Base visitor implementation
├── transaction_listener.go        # Listener interface
├── transaction_base_listener.go   # Base listener implementation
├── Transaction.interp             # ANTLR interpreter data
├── Transaction.tokens             # Token definitions
├── TransactionLexer.interp        # Lexer interpreter data
├── TransactionLexer.tokens        # Lexer token definitions
└── README.md                      # This file
```

## Usage

**Do not use this package directly.** Instead, use the higher-level `transaction` package:

```go
import "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"

// Validate DSL syntax
err := transaction.Validate(dslContent)

// Parse DSL to struct
result := transaction.Parse(dslContent)
tx := result.(libTransaction.Transaction)
```

## Regenerating Parser Code

If you modify the `Transaction.g4` grammar file, regenerate this package:

### Prerequisites

Install ANTLR4:

```bash
# macOS
brew install antlr

# Linux
wget https://www.antlr.org/download/antlr-4.13.1-complete.jar
alias antlr4='java -jar antlr-4.13.1-complete.jar'

# Windows
# Download from https://www.antlr.org/download.html
```

### Regeneration Steps

```bash
# Navigate to gold directory
cd pkg/gold

# Generate parser code
antlr4 -Dlanguage=Go -visitor -listener Transaction.g4 -o parser/

# Verify generated files
ls parser/*.go
```

### Generated Code Options

- `-Dlanguage=Go`: Generate Go code
- `-visitor`: Generate visitor pattern code
- `-listener`: Generate listener pattern code
- `-o parser/`: Output directory

## ANTLR4 Overview

ANTLR (ANother Tool for Language Recognition) is a powerful parser generator that:

- Reads grammar files (`.g4`)
- Generates lexer and parser code
- Provides visitor and listener patterns for tree traversal
- Handles error recovery and reporting

## Grammar File

The grammar is defined in `../Transaction.g4`:

```antlr
grammar Transaction;

// Tokens
VERSION: 'V1';
INT: [0-9]+;
STRING: '"' .*? '"';
UUID: [a-zA-Z0-9_\-/]+;
REMAINING: ':remaining';
VARIABLE: '$'[a-zA-Z0-9_\-]*;
ACCOUNT: '@'[a-zA-Z0-9_\-/]*;
WS: [ \t\r\n]+ -> skip;

// Rules
transaction: '(' ('transaction' | 'transaction-template') VERSION ... ')';
// ... more rules
```

## Visitor Pattern

The generated visitor code provides methods for each grammar rule:

```go
type TransactionVisitor interface {
    VisitTransaction(ctx *TransactionContext) interface{}
    VisitSend(ctx *SendContext) interface{}
    VisitSource(ctx *SourceContext) interface{}
    VisitDistribute(ctx *DistributeContext) interface{}
    // ... more visit methods
}
```

Our custom visitor (`transaction.TransactionVisitor`) implements this interface.

## Listener Pattern

The generated listener code provides enter/exit methods:

```go
type TransactionListener interface {
    EnterTransaction(ctx *TransactionContext)
    ExitTransaction(ctx *TransactionContext)
    // ... more enter/exit methods
}
```

Our custom listener (`transaction.TransactionListener`) implements this interface.

## Parse Tree Structure

The parser generates a tree structure:

```
TransactionContext
├── ChartOfAccountsGroupNameContext
├── DescriptionContext (optional)
├── CodeContext (optional)
├── PendingContext (optional)
├── MetadataContext (optional)
└── SendContext
    ├── SourceContext
    │   └── FromContext (1 or more)
    └── DistributeContext
        └── ToContext (1 or more)
```

## Token Types

The lexer generates these token types:

- `VERSION`: Version identifier (V1)
- `INT`: Integer literals
- `STRING`: String literals
- `UUID`: UUID/identifier tokens
- `REMAINING`: :remaining keyword
- `VARIABLE`: Variable references ($var)
- `ACCOUNT`: Account aliases (@account)
- `WS`: Whitespace (skipped)

## Context Objects

Each grammar rule has a corresponding context object:

- `TransactionContext`: Root transaction
- `SendContext`: Send specification
- `SourceContext`: Source accounts
- `DistributeContext`: Destination accounts
- `FromContext`: Individual source
- `ToContext`: Individual destination
- `AmountContext`: Fixed amount
- `ShareIntContext`: Percentage share
- `RemainingContext`: Remaining balance

## Error Handling

The generated parser provides error handling:

- `ErrorListener` interface for collecting errors
- `RecognitionException` for syntax errors
- Error recovery strategies

Our custom error listener (`transaction.Error`) collects all errors.

## Performance

### Parser Performance

- Lexing: O(n) where n is DSL length
- Parsing: O(n) for most grammars
- Tree building: Additional overhead
- Visitor traversal: O(nodes)

### Optimization Tips

- Reuse lexer/parser instances when possible (not currently implemented)
- Disable parse tree building if only validating (`p.BuildParseTrees = false`)
- Use listener instead of visitor for memory efficiency

## Maintenance Notes

### Do Not Edit Generated Files

All `.go` files in this directory are generated by ANTLR. Manual edits will be lost when regenerating.

**To make changes:**

1. Edit `Transaction.g4` grammar file
2. Regenerate parser code
3. Update custom visitor/listener in `transaction/` package

### Version Compatibility

- ANTLR4 version: 4.13.1
- Go runtime: github.com/antlr4-go/antlr/v4
- Grammar version: V1

### Testing Generated Code

The generated code is tested indirectly through `transaction/` package tests.

## Troubleshooting

### Parser Generation Fails

- Ensure ANTLR4 is installed correctly
- Check grammar syntax in Transaction.g4
- Verify output directory exists

### Runtime Errors

- Ensure antlr4-go runtime version matches generator version
- Check for breaking changes in ANTLR releases
- Verify grammar file is compatible with Go target

### Parse Errors

- Check DSL syntax matches grammar
- Use Validate() to get detailed error messages
- Refer to Transaction.g4 for grammar rules

## References

- ANTLR4 Documentation: https://www.antlr.org/
- ANTLR4 Go Target: https://github.com/antlr/antlr4/blob/master/doc/go-target.md
- Grammar File: `../Transaction.g4`
- Custom Implementation: `../transaction/`

## Version History

This package is regenerated as needed when the grammar changes.

**Last Generated:** Check git history for generation date
**ANTLR Version:** 4.13.1
**Grammar Version:** V1
