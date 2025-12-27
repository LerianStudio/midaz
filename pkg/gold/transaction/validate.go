package transaction

import (
	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	"github.com/antlr4-go/antlr/v4"
)

// TransactionListener is an ANTLR listener for transaction parse trees.
// It validates the structure of parsed transactions.
type TransactionListener struct {
	*parser.BaseTransactionListener
}

// NewTransaction creates a new TransactionListener instance.
func NewTransaction() *TransactionListener {
	return new(TransactionListener)
}

// Validate validates a transaction DSL string and returns any compilation errors.
// It returns nil if the DSL is valid, or an Error object containing syntax errors.
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
