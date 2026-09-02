// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildTransaction_OperationalTypeCodePropagation proves BuildTransaction copies
// the operationalTypeCode field verbatim onto the canonical carrier; a forgotten copy
// silently drops the field (HIGH RISK), so both the present and absent cases are asserted.
func TestBuildTransaction_OperationalTypeCodePropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "absent stays empty", code: ""},
		{name: "present is copied", code: "PIX_IN"},
		{name: "max length code is copied", code: "TED_OUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := CreateTransactionInput{OperationalTypeCode: tt.code}

			result := input.BuildTransaction()

			require.NotNil(t, result)
			assert.Equal(t, tt.code, result.OperationalTypeCode,
				"BuildTransaction must copy OperationalTypeCode verbatim")
		})
	}
}

// TestBuildInflowEntry_OperationalTypeCodePropagation proves BuildInflowEntry copies
// operationalTypeCode onto the canonical carrier.
func TestBuildInflowEntry_OperationalTypeCodePropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "absent stays empty", code: ""},
		{name: "present is copied", code: "PIX_IN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := CreateTransactionInflowInput{OperationalTypeCode: tt.code}

			result := input.BuildInflowEntry()

			require.NotNil(t, result)
			assert.Equal(t, tt.code, result.OperationalTypeCode,
				"BuildInflowEntry must copy OperationalTypeCode verbatim")
		})
	}
}

// TestBuildOutflowEntry_OperationalTypeCodePropagation proves BuildOutflowEntry copies
// operationalTypeCode onto the canonical carrier.
func TestBuildOutflowEntry_OperationalTypeCodePropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "absent stays empty", code: ""},
		{name: "present is copied", code: "PIX_OUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := CreateTransactionOutflowInput{OperationalTypeCode: tt.code}

			result := input.BuildOutflowEntry()

			require.NotNil(t, result)
			assert.Equal(t, tt.code, result.OperationalTypeCode,
				"BuildOutflowEntry must copy OperationalTypeCode verbatim")
		})
	}
}

// TestTransaction_OperationalTypeCode_OmittedWhenAbsent proves ADR-002 byte-identical:
// an absent code marshals to a body with no operationalTypeCode key, so the persisted
// body JSONB and the idempotency hash source stay byte-identical to pre-2.2.2 bodies.
func TestTransaction_OperationalTypeCode_OmittedWhenAbsent(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(Transaction{})
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	assert.NotContains(t, asMap, "operationalTypeCode",
		"an absent operationalTypeCode must be omitted from the transaction body")
}

// TestTransaction_OperationalTypeCode_PersistsInBody proves the code survives a JSON
// round-trip through the carrier — the mechanism that carries it in the body JSONB across
// pending -> commit/cancel re-resolution (ADR-004: reloaded, never re-evaluated).
func TestTransaction_OperationalTypeCode_PersistsInBody(t *testing.T) {
	t.Parallel()

	original := Transaction{OperationalTypeCode: "PIX_IN"}

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))
	require.Contains(t, asMap, "operationalTypeCode",
		"a populated operationalTypeCode must appear in the transaction body")
	assert.Equal(t, "PIX_IN", asMap["operationalTypeCode"])

	var reloaded Transaction
	require.NoError(t, json.Unmarshal(raw, &reloaded))
	assert.Equal(t, "PIX_IN", reloaded.OperationalTypeCode,
		"operationalTypeCode must survive the body round-trip that commit/cancel replays")
}
