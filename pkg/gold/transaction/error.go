// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"github.com/antlr4-go/antlr/v4"
)

// Error holds DSL compilation errors collected during lexing and parsing phases.
type Error struct {
	*antlr.DefaultErrorListener
	Errors []CompileError
	Source string
}

// CompileError represents a single compilation error with location and message details.
type CompileError struct {
	Line    int
	Column  int
	Message string
	Source  string
}

// SyntaxError implements the antlr.ErrorListener interface to collect syntax errors.
func (t *Error) SyntaxError(recognizer antlr.Recognizer, offendingSymbol any, line, column int, msg string, e antlr.RecognitionException) {
	t.Errors = append(t.Errors, CompileError{
		Line:    line,
		Column:  column,
		Message: msg,
	})
}
