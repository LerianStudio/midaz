// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateOperationDirection covers the direction-field validator on async
// operations. The function is the wire-format guardrail used by the bulk consumer:
// pre-migration v3.5.3 payloads omit the direction (warning, not error), and current
// payloads must be one of the two canonical values. Anything else is a hard error.
func TestValidateOperationDirection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		direction string
		wantErr   bool
		// expectedMsgFragment is checked via Contains when wantErr is true.
		expectedMsgFragment string
	}{
		{
			name:      "empty direction is allowed (v3.5.3 backward compatibility)",
			direction: "",
			wantErr:   false,
		},
		{
			name:      "lowercase debit is valid",
			direction: "debit",
			wantErr:   false,
		},
		{
			name:      "lowercase credit is valid",
			direction: "credit",
			wantErr:   false,
		},
		{
			name:      "uppercase DEBIT is valid (case-insensitive)",
			direction: "DEBIT",
			wantErr:   false,
		},
		{
			name:      "uppercase CREDIT is valid (case-insensitive)",
			direction: "CREDIT",
			wantErr:   false,
		},
		{
			name:      "mixed-case Debit is valid (case-insensitive)",
			direction: "Debit",
			wantErr:   false,
		},
		{
			name:                "garbage value is rejected",
			direction:           "sideways",
			wantErr:             true,
			expectedMsgFragment: "must be 'debit' or 'credit'",
		},
		{
			name:                "non-direction string is rejected",
			direction:           "transfer",
			wantErr:             true,
			expectedMsgFragment: "invalid direction",
		},
		{
			name:                "whitespace-only is rejected (not equal to empty after ToLower)",
			direction:           " ",
			wantErr:             true,
			expectedMsgFragment: "invalid direction",
		},
	}

	logger := libLog.NewNop()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			oper := &operation.Operation{
				ID:        "op-123",
				Direction: tc.direction,
			}

			err := validateOperationDirection(context.Background(), logger, oper)

			if !tc.wantErr {
				require.NoError(t, err, "direction %q should be accepted", tc.direction)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedMsgFragment, "error message must identify the failure")
			assert.Contains(t, err.Error(), "op-123", "error must include the operation ID for debugging")
		})
	}
}
