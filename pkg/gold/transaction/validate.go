// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"github.com/antlr4-go/antlr/v4"

	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
)

// TransactionListener implements the ANTLR parse tree listener for transaction DSL validation.
type TransactionListener struct {
	*parser.BaseTransactionListener
}

// NewTransaction creates a new TransactionListener for DSL validation.
func NewTransaction() *TransactionListener {
	return new(TransactionListener)
}

// Validate parses the given DSL string and returns any lexer or parser errors found.
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
