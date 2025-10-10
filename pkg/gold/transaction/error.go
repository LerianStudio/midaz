// Package transaction provides parsing and validation for the Gold DSL.
// This file contains error handling structures for DSL syntax errors.
package transaction

import (
	"github.com/antlr4-go/antlr/v4"
)

// Error represents a collection of syntax errors from DSL parsing.
//
// This struct implements antlr.ErrorListener to collect all syntax errors that occur
// during lexing or parsing. It stores both the errors and the source (lexer or parser)
// where the errors occurred.
type Error struct {
	*antlr.DefaultErrorListener
	// A collection of syntax errors found during parsing.
	Errors []CompileError
	// The source of the errors, either "lexer" or "parser".
	Source string
}

// CompileError represents a single syntax error in the DSL.
//
// This struct captures the location and description of a syntax error, allowing
// clients to provide detailed error messages to users.
type CompileError struct {
	// The line number where the error occurred (1-indexed).
	Line int
	// The column number where the error occurred (0-indexed).
	Column int
	// A detailed error message from the ANTLR parser.
	Message string
	// The source of the error.
	Source string
}

// SyntaxError is called by ANTLR when a syntax error is encountered.
//
// This method implements the antlr.ErrorListener interface. It's called automatically
// by ANTLR during parsing whenever a syntax error is detected. The method collects
// all errors so they can be reported to the user.
//
// Parameters:
//   - recognizer: The ANTLR recognizer (lexer or parser) that encountered the error.
//   - offendingSymbol: The token or symbol that caused the error.
//   - line: The line number where the error occurred (1-indexed).
//   - column: The column number where the error occurred (0-indexed).
//   - msg: A detailed error message from ANTLR.
//   - e: The recognition exception, which may be nil.
func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
