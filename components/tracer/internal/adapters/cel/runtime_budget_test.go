// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"strings"
	"testing"

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestAdapterRuntimeCostExceedsStaticEstimate(t *testing.T) {
	t.Parallel()
	adapter := newTestAdapterWithCostLimit(t, 100)
	program, err := adapter.Compile(context.Background(), `metadata["payload"].contains("z")`)
	require.NoError(t, err)

	req := newTestRequest()
	req.Metadata = map[string]any{"payload": strings.Repeat("a", 20000)}
	matched, err := adapter.Evaluate(context.Background(), program, req)
	require.ErrorIs(t, err, constant.ErrExpressionCostExceeded)
	require.ErrorIs(t, err, constant.ErrExpressionEvaluation)
	require.False(t, matched)

	// A budget belongs to one execution; exhaustion must not poison the cached program.
	req.Metadata = map[string]any{"payload": "z"}
	matched, err = adapter.Evaluate(context.Background(), program, req)
	require.NoError(t, err)
	require.True(t, matched)
}

func TestAdapterHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	adapter := newTestAdapter(t)
	program, err := adapter.Compile(context.Background(), "true")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	matched, err := adapter.Evaluate(ctx, program, newTestRequest())
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, matched)

	_, err = adapter.Compile(ctx, "true")
	require.ErrorIs(t, err, context.Canceled)
}

func TestAdapterInterruptsRunningComprehension(t *testing.T) {
	t.Parallel()
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	visited := 0
	env, err := adapter.env.CELEnv().Extend(celgo.Function("step",
		celgo.Overload("step_int", []*celgo.Type{celgo.IntType}, celgo.BoolType,
			celgo.UnaryBinding(func(_ ref.Val) ref.Val {
				visited++
				cancel()

				return types.True
			}))))
	require.NoError(t, err)
	adapter.env = &Environment{env: env}
	program, err := adapter.Compile(context.Background(), `metadata["items"].all(x, step(x))`)
	require.NoError(t, err)
	req := newTestRequest()
	items := make([]int, 10000)
	req.Metadata = map[string]any{"items": items}
	matched, err := adapter.Evaluate(ctx, program, req)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, matched)
	require.Positive(t, visited)
	require.Less(t, visited, len(items), "cancel must interrupt iteration, not just discard its final result")
}
