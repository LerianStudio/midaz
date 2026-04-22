// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOverdraftErrorCodes_Registered verifies that the three overdraft
// error codes are registered in the sentinel error table with the exact
// numeric codes for the overdraft feature.
func TestOverdraftErrorCodes_Registered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sentinel    error
		expected    string
		description string
	}{
		{
			name:        "ErrOverdraftLimitExceeded is code 0167",
			sentinel:    ErrOverdraftLimitExceeded,
			expected:    "0167",
			description: "transaction would exceed the balance's overdraft limit",
		},
		{
			name:        "ErrDirectOperationOnInternalBalance is code 0168",
			sentinel:    ErrDirectOperationOnInternalBalance,
			expected:    "0168",
			description: "direct operation attempted on an internal-scope balance",
		},
		{
			name:        "ErrDeletionOfInternalBalance is code 0169",
			sentinel:    ErrDeletionOfInternalBalance,
			expected:    "0169",
			description: "attempt to delete an internal-scope balance",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, tt.sentinel, "%s: sentinel must be registered", tt.description)
			assert.Equal(t, tt.expected, tt.sentinel.Error(),
				"%s: sentinel error code must equal %s", tt.description, tt.expected)
		})
	}
}
