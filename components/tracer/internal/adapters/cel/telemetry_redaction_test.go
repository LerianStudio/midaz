// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// The precision errors name the class, not the amount the caller sent.
func TestBuildActivation_PrecisionErrorOmitsAmount(t *testing.T) {
	t.Parallel()

	for _, amount := range []string{"9007199254740993", "-9007199254740993", "1e400"} {
		req := newTestRequest()
		req.Amount = decimal.RequireFromString(amount)

		_, err := BuildActivation(req)
		require.ErrorIs(t, err, constant.ErrAmountExceedsPrecision)
		assert.NotContains(t, err.Error(), req.Amount.String(), "precision error leaks the amount")
		assert.NotContains(t, err.Error(), amount, "precision error leaks the amount")
	}
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestAdapter_Evaluate_SpanEventsCarryClassNotText(t *testing.T) {
	const secret = "secret-metadata-value"

	for _, tc := range []struct {
		name       string
		expression string
		amount     string
		metadata   map[string]any
		leak       string
		wantClass  string
	}{
		{
			name:       "amount beyond CEL precision",
			expression: "amount > 0",
			amount:     "9007199254740993",
			leak:       "9007199254740993",
			wantClass:  constant.ErrAmountExceedsPrecision.Error(),
		},
		{
			name:       "runtime failure quoting a metadata value",
			expression: `int(metadata["code"]) > 0`,
			amount:     "100",
			metadata:   map[string]any{"code": secret},
			leak:       secret,
			wantClass:  constant.ErrExpressionEvaluation.Error(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracing := testutil.SetupTestTracing(t)
			ctx := context.Background()
			adapter := newTestAdapter(t)

			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := newTestRequest()
			req.Amount = decimal.RequireFromString(tc.amount)
			req.Metadata = tc.metadata

			_, err = adapter.Evaluate(ctx, program, req)
			require.Error(t, err)

			var recorded bool

			for _, span := range tracing.GetSpans() {
				if span.Name != "adapter.cel.evaluate" {
					continue
				}

				for _, event := range span.Events {
					for _, attr := range event.Attributes {
						assert.NotContains(t, attr.Value.Emit(), tc.leak, "event %q leaks the error text", event.Name)

						if attr.Key == "error" {
							recorded = true

							assert.Equal(t, tc.wantClass, attr.Value.AsString())
						}
					}
				}
			}

			assert.True(t, recorded, "the failure must be recorded on the span")
		})
	}
}
