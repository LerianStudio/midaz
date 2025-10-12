package transaction

import (
	"github.com/antlr4-go/antlr/v4"
)

// Error aggregates compile-time lexer/parser errors for Gold DSL validation.
type Error struct {
	*antlr.DefaultErrorListener
	Errors []CompileError
	Source string
}

// CompileError represents a single syntax error occurrence.
type CompileError struct {
	Line    int
	Column  int
	Message string
	Source  string
}

// SyntaxError implements the antlr.ErrorListener interface and collects
// syntax errors during lexing or parsing.
func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
