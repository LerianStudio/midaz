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

// TestNewTransactionV2_ProjectsOperationalTypeCode proves newTransactionV2 copies
// the canonical operationalTypeCode onto the /v2 wire shape unchanged.
func TestNewTransactionV2_ProjectsOperationalTypeCode(t *testing.T) {
	t.Parallel()

	canonical := buildCanonicalTransactionFixture()
	canonical.OperationalTypeCode = "PIX_IN"

	got := newTransactionV2(canonical)

	require.NotNil(t, got)
	assert.Equal(t, canonical.OperationalTypeCode, got.OperationalTypeCode)
}

// TestTransactionV2_OperationalTypeCode_OmittedWhenAbsent proves the /v2 wire body
// omits operationalTypeCode when it is empty (the frozen contract: present only
// when an account exception applied a type), and carries it when populated.
func TestTransactionV2_OperationalTypeCode_OmittedWhenAbsent(t *testing.T) {
	t.Parallel()

	t.Run("absent when empty", func(t *testing.T) {
		t.Parallel()

		canonical := buildCanonicalTransactionFixture()
		canonical.OperationalTypeCode = ""

		raw, err := json.Marshal(newTransactionV2(canonical))
		require.NoError(t, err)

		var asMap map[string]any
		require.NoError(t, json.Unmarshal(raw, &asMap))

		assert.NotContains(t, asMap, "operationalTypeCode",
			"an empty operationalTypeCode must be omitted from the v2 wire body")
	})

	t.Run("present when populated", func(t *testing.T) {
		t.Parallel()

		canonical := buildCanonicalTransactionFixture()
		canonical.OperationalTypeCode = "PIX_OUT"

		raw, err := json.Marshal(newTransactionV2(canonical))
		require.NoError(t, err)

		var asMap map[string]any
		require.NoError(t, json.Unmarshal(raw, &asMap))

		require.Contains(t, asMap, "operationalTypeCode",
			"a populated operationalTypeCode must appear on the v2 wire body")
		assert.Equal(t, "PIX_OUT", asMap["operationalTypeCode"])
	})
}

// TestTransactionV1_ProjectsOperationalTypeCode proves the /v1 response shape
// carries operationalTypeCode through the embedded canonical transaction (unlike
// feesSkipped/tracerSkipped, which v1 shadows). Present when populated, omitted
// when empty.
func TestTransactionV1_ProjectsOperationalTypeCode(t *testing.T) {
	t.Parallel()

	canonical := buildCanonicalTransactionFixture()
	canonical.OperationalTypeCode = "PIX_IN"

	raw, err := json.Marshal(newTransactionV1(canonical))
	require.NoError(t, err)

	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))

	require.Contains(t, asMap, "operationalTypeCode",
		"the v1 body must carry operationalTypeCode through the embedded transaction")
	assert.Equal(t, "PIX_IN", asMap["operationalTypeCode"])
}
