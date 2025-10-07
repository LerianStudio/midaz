// Package transaction provides parsing and validation for the Gold DSL.
// This file contains validation functions for checking DSL syntax.
package transaction

import (
	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	"github.com/antlr4-go/antlr/v4"
)

// TransactionListener implements the ANTLR listener pattern for Gold DSL validation.
//
// This listener is used during the validation phase to walk the parse tree.
// Currently, it doesn't perform any custom actions during the walk, but it's
// available for future enhancements like semantic validation or AST transformation.
type TransactionListener struct {
	*parser.BaseTransactionListener
}

// NewTransaction creates a new TransactionListener instance.
//
// Returns:
//   - *TransactionListener: A new listener ready to walk parse trees
func NewTransaction() *TransactionListener {
	return new(TransactionListener)
}

// Validate validates Gold DSL syntax without constructing the transaction object.
//
// This function performs syntax validation only, checking that the DSL conforms to
// the Gold grammar defined in Transaction.g4. It does not perform semantic validation
// (e.g., checking if accounts exist, validating balances).
//
// The validation process:
// 1. Lexical analysis: Converts DSL text into tokens
// 2. Syntax analysis: Parses tokens according to grammar rules
// 3. Error collection: Gathers all syntax errors found
//
// The function uses custom error listeners to collect all errors rather than stopping
// at the first error, allowing users to see all syntax issues at once.
//
// Parameters:
//   - dsl: Gold DSL string to validate
//
// Returns:
//   - *Error: Error struct containing all syntax errors, or nil if valid
//   - If errors from lexer: Error.Source = "lexer"
//   - If errors from parser: Error.Source = "parser"
//
// Example:
//
//	dslContent := `(transaction V1 ...)`
//	if err := transaction.Validate(dslContent); err != nil {
//	    for _, compileErr := range err.Errors {
//	        fmt.Printf("Line %d, Column %d: %s\n",
//	            compileErr.Line, compileErr.Column, compileErr.Message)
//	    }
//	    return constant.ErrInvalidScriptFormat
//	}
//	// DSL is syntactically valid, proceed with Parse()
//	tx := transaction.Parse(dslContent)
//
// Use Case:
//   - Validate DSL before storing in database
//   - Provide immediate feedback on syntax errors
//   - Separate syntax validation from semantic validation
func Validate(dsl string) *Error {
	lexerErrors := &Error{}
	input := antlr.NewInputStream(dsl)
	lexer := parser.NewTransactionLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(lexerErrors)

	parserErrors := &Error{}
	p := parser.NewTransactionParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(parserErrors)
	p.BuildParseTrees = true
	p.AddErrorListener(antlr.NewDiagnosticErrorListener(true))

	antlr.ParseTreeWalkerDefault.Walk(NewTransaction(), p.Transaction())

	if len(lexerErrors.Errors) > 0 {
		lexerErrors.Source = "lexer"
		return lexerErrors
	}

	if len(parserErrors.Errors) > 0 {
		parserErrors.Source = "parser"
		return parserErrors
	}

	return nil
}
