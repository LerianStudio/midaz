package transaction

import (
	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	"github.com/antlr4-go/antlr/v4"
)

// TransactionListener captures lexer and parser errors produced during
// validation of Gold DSL input.
type TransactionListener struct {
	*parser.BaseTransactionListener
}

// NewTransaction creates a new TransactionListener instance.
func NewTransaction() *TransactionListener {
	return new(TransactionListener)
}

// Validate checks a Gold DSL string for syntax errors. It returns an Error with
// details when invalid, or nil when the input is valid.
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
