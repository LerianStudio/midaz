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
// available for future enhancements like semantic validation.
type TransactionListener struct {
	*parser.BaseTransactionListener
}

// NewTransactionListener creates a new TransactionListener instance.
func NewTransactionListener() *TransactionListener {
	return new(TransactionListener)
}

// Validate validates Gold DSL syntax without constructing the transaction object.
//
// This function performs syntax validation only, checking that the DSL conforms to
// the Gold grammar defined in Transaction.g4. It does not perform semantic validation
// (e.g., checking if accounts exist, validating balances).
//
// The function uses custom error listeners to collect all errors rather than stopping
// at the first error, allowing users to see all syntax issues at once.
//
// Parameters:
//   - dsl: The Gold DSL string to validate.
//
// Returns:
//   - *Error: An Error struct containing all syntax errors, or nil if the DSL is valid.
//     The Source field of the Error struct will be "lexer" or "parser".
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
	// FIXME: This diagnostic listener prints to stderr, which might not be desirable in a library.
	// Consider removing it or making it configurable to avoid unexpected console output.
	p.AddErrorListener(antlr.NewDiagnosticErrorListener(true))

	antlr.ParseTreeWalkerDefault.Walk(NewTransactionListener(), p.Transaction())

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
