// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=expression_compiler.go -destination=mocks/expression_compiler_mock.go -package=mocks

import "context"

// ExpressionCompiler compiles CEL expression strings into executable programs.
// Defined in workers package to avoid importing the CEL adapter directly.
// The returned value is typed as `any` to decouple from cel.Program.
// Satisfied by bootstrap's celCompilerAdapter (config.go:84-91).
type ExpressionCompiler interface {
	Compile(ctx context.Context, expression string) (any, error)
}
