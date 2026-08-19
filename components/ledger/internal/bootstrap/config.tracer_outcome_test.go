// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTracerOutcomeModeDefaultsLegacyAndRequiresExplicitV2(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "legacy", " LEGACY "} {
		mode, err := normalizeTracerOutcomeMode(input)
		require.NoError(t, err)
		assert.Equal(t, tracerOutcomeModeLegacy, mode)
	}
	mode, err := normalizeTracerOutcomeMode("LEDGER_OUTCOME_V2")
	require.NoError(t, err)
	assert.Equal(t, tracerOutcomeModeV2, mode)

	_, err = normalizeTracerOutcomeMode("automatic")
	require.ErrorContains(t, err, "invalid TRACER_OUTCOME_MODE")
}
