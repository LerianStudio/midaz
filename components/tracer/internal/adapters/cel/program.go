// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package cel provides CEL expression compilation and evaluation.
package cel

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/cel-go/cel"
)

// DefaultCostLimit is the default CEL expression cost limit.
const DefaultCostLimit uint64 = 10000

// CompiledProgram represents a compiled CEL expression with metadata.
type CompiledProgram struct {
	// ExpressionHash is the SHA-256 hash of the source expression.
	ExpressionHash string

	// SourceExpression is the original CEL expression string.
	SourceExpression string

	// Program is the compiled CEL program ready for evaluation.
	Program cel.Program

	// CompiledAt is the timestamp when the expression was compiled.
	CompiledAt time.Time

	// CompileTimeMs is the compilation duration in milliseconds.
	CompileTimeMs int64
}

// HashExpression computes the SHA-256 hash of an expression string.
func HashExpression(expression string) string {
	h := sha256.Sum256([]byte(expression))
	return hex.EncodeToString(h[:])
}
