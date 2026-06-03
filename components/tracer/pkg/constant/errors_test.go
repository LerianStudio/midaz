// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExpressionErrors_AreSentinelErrors verifies CEL errors are defined as sentinel errors.
func TestExpressionErrors_AreSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "ErrExpressionSyntax is sentinel error",
			err:      ErrExpressionSyntax,
			expected: "TRC-0083",
		},
		{
			name:     "ErrExpressionType is sentinel error",
			err:      ErrExpressionType,
			expected: "TRC-0084",
		},
		{
			name:     "ErrExpressionCostExceeded is sentinel error",
			err:      ErrExpressionCostExceeded,
			expected: "TRC-0085",
		},
		{
			name:     "ErrExpressionEvaluation is sentinel error",
			err:      ErrExpressionEvaluation,
			expected: "TRC-0086",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.err, "Error should not be nil")
			assert.Equal(t, tc.expected, tc.err.Error(), "Error message should match code")
		})
	}
}

// TestExpressionErrors_CanBeWrapped verifies CEL errors can be wrapped with additional context.
func TestExpressionErrors_CanBeWrapped(t *testing.T) {
	tests := []struct {
		name        string
		baseErr     error
		wrapMessage string
	}{
		{
			name:        "ErrExpressionSyntax can be wrapped",
			baseErr:     ErrExpressionSyntax,
			wrapMessage: "invalid syntax at line 1",
		},
		{
			name:        "ErrExpressionType can be wrapped",
			baseErr:     ErrExpressionType,
			wrapMessage: "expression returns string instead of bool",
		},
		{
			name:        "ErrExpressionCostExceeded can be wrapped",
			baseErr:     ErrExpressionCostExceeded,
			wrapMessage: "cost 15000 exceeds limit 10000",
		},
		{
			name:        "ErrExpressionEvaluation can be wrapped",
			baseErr:     ErrExpressionEvaluation,
			wrapMessage: "division by zero",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrappedErr := fmt.Errorf("%s: %w", tc.wrapMessage, tc.baseErr)

			assert.NotNil(t, wrappedErr, "Wrapped error should not be nil")
			assert.Contains(t, wrappedErr.Error(), tc.wrapMessage, "Wrapped error should contain message")
			assert.Contains(t, wrappedErr.Error(), tc.baseErr.Error(), "Wrapped error should contain base error")
		})
	}
}

// TestExpressionErrors_ErrorsIs verifies errors.Is works with CEL errors.
func TestExpressionErrors_ErrorsIs(t *testing.T) {
	tests := []struct {
		name    string
		baseErr error
	}{
		{
			name:    "errors.Is works with ErrExpressionSyntax",
			baseErr: ErrExpressionSyntax,
		},
		{
			name:    "errors.Is works with ErrExpressionType",
			baseErr: ErrExpressionType,
		},
		{
			name:    "errors.Is works with ErrExpressionCostExceeded",
			baseErr: ErrExpressionCostExceeded,
		},
		{
			name:    "errors.Is works with ErrExpressionEvaluation",
			baseErr: ErrExpressionEvaluation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Wrap the error
			wrappedErr := fmt.Errorf("context: %w", tc.baseErr)

			// errors.Is should find the base error
			assert.True(t, errors.Is(wrappedErr, tc.baseErr),
				"errors.Is should return true for wrapped error")

			// errors.Is should return false for different error
			assert.False(t, errors.Is(wrappedErr, ErrBadRequest),
				"errors.Is should return false for different error")
		})
	}
}
