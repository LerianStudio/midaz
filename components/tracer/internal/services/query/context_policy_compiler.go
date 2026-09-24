// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ContextPolicyCompiler must have immutable CEL environment/resource settings
// for its lifetime. Reconfiguration creates a new compiler and cache instance.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=context_policy_compiler.go -destination=compiledmocks/context_policy_compiler_mock.go -package=compiledmocks
type ContextPolicyCompiler interface {
	Compile(context.Context, model.ContextPolicy) (*CompiledContextPolicy, error)
}
