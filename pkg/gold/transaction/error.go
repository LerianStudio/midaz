package transaction

import (
	"github.com/antlr4-go/antlr/v4"
)

// \1 represents an entity
type Error struct {
	*antlr.DefaultErrorListener
	Errors []CompileError
	Source string
}

// \1 represents an entity
type CompileError struct {
	Line    int
	Column  int
	Message string
	Source  string
}

// func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) { performs an operation
func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
