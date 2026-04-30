// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperationSnapshot_JSONMarshal verifies that OperationSnapshot serializes
// to JSON using camelCase keys with both fields ALWAYS present. Under the
// always-populated wire-shape contract, non-overdraft operations carry "0" /
// "0" rather than absent fields, so the wire shape is uniform across the
// entire ledger.
func TestOperationSnapshot_JSONMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot OperationSnapshot
		want     string
	}{
		{
			name: "non-overdraft op carries zero values explicitly",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
			want: `{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`,
		},
		{
			name: "active overdraft populated with non-zero before and after",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "50.00",
				OverdraftUsedAfter:  "130.00",
			},
			want: `{"overdraftUsedBefore":"50.00","overdraftUsedAfter":"130.00"}`,
		},
		{
			name: "debit split: zero before, non-zero after",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "50.00",
			},
			want: `{"overdraftUsedBefore":"0","overdraftUsedAfter":"50.00"}`,
		},
		{
			name: "credit repayment: non-zero before, zero after",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "130.00",
				OverdraftUsedAfter:  "0",
			},
			want: `{"overdraftUsedBefore":"130.00","overdraftUsedAfter":"0"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tc.snapshot)
			require.NoError(t, err, "marshal must not fail")
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

// TestOperationSnapshot_JSONUnmarshal verifies that OperationSnapshot
// deserializes from JSON. Unlike pre-reversal behaviour, missing keys decode
// to the empty string rather than nil — the always-populated contract is
// applied at the persistence-layer mappers (ToEntity / OperationFromRedis),
// not here. This test pins the raw JSON ↔ struct mapping.
func TestOperationSnapshot_JSONUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantBefore string
		wantAfter  string
	}{
		{
			name:       "both fields populated round-trip",
			input:      `{"overdraftUsedBefore":"50.00","overdraftUsedAfter":"130.00"}`,
			wantBefore: "50.00",
			wantAfter:  "130.00",
		},
		{
			name:       "explicit zero values round-trip",
			input:      `{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`,
			wantBefore: "0",
			wantAfter:  "0",
		},
		{
			name:       "empty object decodes to empty strings (legacy row shape)",
			input:      `{}`,
			wantBefore: "",
			wantAfter:  "",
		},
		{
			name:       "only overdraftUsedAfter present (legacy partial row)",
			input:      `{"overdraftUsedAfter":"130.00"}`,
			wantBefore: "",
			wantAfter:  "130.00",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got OperationSnapshot
			err := json.Unmarshal([]byte(tc.input), &got)
			require.NoError(t, err, "unmarshal must not fail")

			assert.Equal(t, tc.wantBefore, got.OverdraftUsedBefore)
			assert.Equal(t, tc.wantAfter, got.OverdraftUsedAfter)
		})
	}
}

// TestOperationSnapshot_RoundTrip verifies that marshal→unmarshal is an
// identity operation for representative shapes under the always-populated
// contract.
func TestOperationSnapshot_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snapshot OperationSnapshot
	}{
		{
			name: "active overdraft round-trips",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "50.00",
				OverdraftUsedAfter:  "130.00",
			},
		},
		{
			name: "non-overdraft (both zero) round-trips",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
		},
		{
			name: "credit repayment (after=0) round-trips",
			snapshot: OperationSnapshot{
				OverdraftUsedBefore: "130.00",
				OverdraftUsedAfter:  "0",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tc.snapshot)
			require.NoError(t, err, "marshal must not fail")

			var roundTripped OperationSnapshot
			err = json.Unmarshal(data, &roundTripped)
			require.NoError(t, err, "unmarshal must not fail")

			assert.Equal(t, tc.snapshot.OverdraftUsedBefore, roundTripped.OverdraftUsedBefore)
			assert.Equal(t, tc.snapshot.OverdraftUsedAfter, roundTripped.OverdraftUsedAfter)
		})
	}
}
