package transaction

import (
	"github.com/antlr4-go/antlr/v4"
)

type Error struct {
	*antlr.DefaultErrorListener
	Errors []CompileError
	Source string
}

type CompileError struct {
	Line    int
	Column  int
	Message string
	Source  string
}

func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
