// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// TestIsMissingKeyError_DirectDetection verifies the helper recognises the
// stable cel-go "no such key: ..." error message at the top of the chain and
// when wrapped behind other errors.
func TestIsMissingKeyError_DirectDetection(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "missing key direct",
			err:      errors.New("no such key: channel"),
			expected: true,
		},
		{
			name:     "missing key wrapped once",
			err:      fmt.Errorf("eval failed: %w", errors.New("no such key: caseId")),
			expected: true,
		},
		{
			name:     "missing key wrapped twice with sentinel",
			err:      fmt.Errorf("%w: %w", constant.ErrExpressionEvaluation, errors.New("no such key: channel")),
			expected: true,
		},
		{
			name:     "missing key wrapped behind handler context",
			err:      fmt.Errorf("failed to evaluate expression: %w", fmt.Errorf("%w: %w", constant.ErrExpressionEvaluation, errors.New("no such key: missing"))),
			expected: true,
		},
		{
			name:     "type mismatch error",
			err:      errors.New("no such overload"),
			expected: false,
		},
		{
			name:     "division by zero",
			err:      errors.New("division by zero"),
			expected: false,
		},
		{
			name:     "no such field (different cel-go error)",
			err:      errors.New("no such field"),
			expected: false,
		},
		{
			name:     "no such attribute",
			err:      errors.New("no such attribute(s): metadata.channel"),
			expected: false,
		},
		{
			name:     "syntax error",
			err:      errors.New("ERROR: <input>:1:1: undeclared reference 'foo'"),
			expected: false,
		},
		{
			name:     "joined errors none missing key",
			err:      errors.Join(errors.New("division by zero"), errors.New("no such overload")),
			expected: false,
		},
		{
			name:     "joined errors with missing key inside",
			err:      errors.Join(errors.New("division by zero"), errors.New("no such key: channel")),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMissingKeyError(tt.err)
			assert.Equal(t, tt.expected, got, "input err=%v", tt.err)
		})
	}
}

// TestIsMissingKeyError_RealCELEvaluation guarantees the helper still detects
// the error produced by the actual cel-go runtime, not just by string fixtures.
// This guards against cel-go upgrades that might change the error message.
// Each case exercises a different runtime outcome so we cover both the positive
// (missing key) and negative (other runtime error) classifications end-to-end.
func TestIsMissingKeyError_RealCELEvaluation(t *testing.T) {
	tests := []struct {
		name                 string
		expression           string
		metadata             map[string]any
		expectedIsMissingKey bool
	}{
		{
			name:                 "missing metadata key is classified",
			expression:           `metadata["channel"] == "mobile"`,
			metadata:             map[string]any{"caseId": "case-001"},
			expectedIsMissingKey: true,
		},
		{
			name:                 "division by zero is not classified as missing key",
			expression:           `amount / 0 > 1`,
			metadata:             map[string]any{},
			expectedIsMissingKey: false,
		},
		{
			name:                 "missing metadata key with empty metadata map",
			expression:           `metadata["missing"] == "x"`,
			metadata:             map[string]any{},
			expectedIsMissingKey: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter(t)
			ctx := context.Background()

			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err)

			req := &model.ValidationRequest{
				RequestID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440090"),
				TransactionType: model.TransactionTypePix,
				Amount:          decimal.RequireFromString("100"),
				Currency:        "BRL",
				Account: model.AccountContext{
					ID:     uuid.MustParse("550e8400-e29b-41d4-a716-446655440091"),
					Type:   "checking",
					Status: "active",
				},
				Metadata: tc.metadata,
			}

			_, evalErr := adapter.Evaluate(ctx, program, req)
			require.Error(t, evalErr, "evaluating %q must surface an error", tc.expression)
			assert.Equal(t, tc.expectedIsMissingKey, IsMissingKeyError(evalErr), "classification mismatch for err=%v", evalErr)
			assert.ErrorIs(t, evalErr, constant.ErrExpressionEvaluation, "adapter still wraps with expression evaluation sentinel")
		})
	}
}
