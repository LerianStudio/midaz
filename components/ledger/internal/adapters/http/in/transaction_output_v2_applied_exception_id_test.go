// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTransactionV2_ProjectsAppliedExceptionID proves newTransactionV2 copies
// the canonical appliedExceptionId pointer onto the /v2 wire shape unchanged.
func TestNewTransactionV2_ProjectsAppliedExceptionID(t *testing.T) {
	t.Parallel()

	exceptionID := "88888888-8888-8888-8888-888888888888"

	canonical := buildCanonicalTransactionFixture()
	canonical.AppliedExceptionID = &exceptionID

	got := newTransactionV2(canonical)

	require.NotNil(t, got)
	require.NotNil(t, got.AppliedExceptionID)
	assert.Equal(t, exceptionID, *got.AppliedExceptionID)
}

// TestTransactionV2_AppliedExceptionID_OmittedWhenAbsent proves the /v2 wire body
// omits appliedExceptionId when it is nil (the value is written only when an
// account exception applied), and carries it when populated.
func TestTransactionV2_AppliedExceptionID_OmittedWhenAbsent(t *testing.T) {
	t.Parallel()

	t.Run("absent when nil", func(t *testing.T) {
		t.Parallel()

		canonical := buildCanonicalTransactionFixture()
		canonical.AppliedExceptionID = nil

		raw, err := json.Marshal(newTransactionV2(canonical))
		require.NoError(t, err)

		var asMap map[string]any
		require.NoError(t, json.Unmarshal(raw, &asMap))

		assert.NotContains(t, asMap, "appliedExceptionId",
			"a nil appliedExceptionId must be omitted from the v2 wire body")
	})

	t.Run("present when populated", func(t *testing.T) {
		t.Parallel()

		exceptionID := "99999999-9999-9999-9999-999999999999"

		canonical := buildCanonicalTransactionFixture()
		canonical.AppliedExceptionID = &exceptionID

		raw, err := json.Marshal(newTransactionV2(canonical))
		require.NoError(t, err)

		var asMap map[string]any
		require.NoError(t, json.Unmarshal(raw, &asMap))

		require.Contains(t, asMap, "appliedExceptionId",
			"a populated appliedExceptionId must appear on the v2 wire body")
		assert.Equal(t, exceptionID, asMap["appliedExceptionId"])
	})
}

// TestTransactionV1_ProjectsAppliedExceptionID proves the /v1 response shape
// carries appliedExceptionId through the embedded canonical transaction (v1 gets
// it free via the embed). Present when populated.
func TestTransactionV1_ProjectsAppliedExceptionID(t *testing.T) {
	t.Parallel()

	exceptionID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	canonical := buildCanonicalTransactionFixture()
	canonical.AppliedExceptionID = &exceptionID

	raw, err := json.Marshal(newTransactionV1(canonical))
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	require.Contains(t, asMap, "appliedExceptionId",
		"the v1 body must carry appliedExceptionId through the embedded transaction")
	assert.Equal(t, exceptionID, asMap["appliedExceptionId"])
}
