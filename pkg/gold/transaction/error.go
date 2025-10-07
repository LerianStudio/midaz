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
	*antlr.DefaultErrorListener                // Embeds default error listener
	Errors                      []CompileError // Collection of syntax errors
	Source                      string         // Source of errors: "lexer" or "parser"
}

// CompileError represents a single syntax error in the DSL.
//
// This struct captures the location and description of a syntax error, allowing
// clients to provide detailed error messages to users.
type CompileError struct {
	Line    int    // Line number where the error occurred (1-indexed)
	Column  int    // Column number where the error occurred (0-indexed)
	Message string // Detailed error message from ANTLR
	Source  string // Source of the error (currently unused, but available for future use)
}

// SyntaxError is called by ANTLR when a syntax error is encountered.
//
// This method implements the antlr.ErrorListener interface. It's called automatically
// by ANTLR during parsing whenever a syntax error is detected. The method collects
// all errors so they can be reported to the user.
//
// Parameters:
//   - recognizer: The ANTLR recognizer (lexer or parser) that encountered the error
//   - offendingSymbol: The token or symbol that caused the error
//   - line: Line number where the error occurred (1-indexed)
//   - column: Column number where the error occurred (0-indexed)
//   - msg: Detailed error message from ANTLR
//   - e: Recognition exception (may be nil)
//
// Example Error:
//
//	Line: 5, Column: 12
//	Message: "mismatched input 'USD' expecting {'(', UUID}"
func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
