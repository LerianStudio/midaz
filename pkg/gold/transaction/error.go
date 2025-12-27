package transaction

import (
	"github.com/antlr4-go/antlr/v4"
)

// Error represents a compilation error listener that collects syntax errors from the ANTLR parser.
type Error struct {
	*antlr.DefaultErrorListener
	Errors []CompileError
	Source string
}

// CompileError represents a single syntax error with location and message information.
type CompileError struct {
	Line    int
	Column  int
	Message string
	Source  string
}

// SyntaxError is called by the ANTLR parser when a syntax error is encountered.
// It captures the error details including line, column, and error message.
func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
