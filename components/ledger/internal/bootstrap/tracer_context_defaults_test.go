// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestTracerContextDefaultsPreserveExplicitZero(t *testing.T) {
	for _, key := range []string{"TRACER_CONTEXT_MAX_FRACTION_DIGITS", "TRACER_CONTEXT_MAX_ACCOUNTS"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	cfg := &Config{}
	applyTracerContextDefaults(cfg)
	require.Equal(t, "128", cfg.TracerContextMaxFractionDigits)
	require.Equal(t, tracercontract.DefaultResourceProfile().Facts.MaxAccounts, cfg.TracerContextMaxAccounts)
	require.False(t, cfg.TracerContextEnabled)
	require.Empty(t, cfg.TracerIntegrationID)
	require.Empty(t, cfg.TracerAssetNamespace)
	t.Setenv("TRACER_CONTEXT_MAX_FRACTION_DIGITS", "0")
	t.Setenv("TRACER_CONTEXT_MAX_ACCOUNTS", "0")
	cfg = &Config{TracerContextMaxFractionDigits: "0"}
	applyTracerContextDefaults(cfg)
	require.Equal(t, "0", cfg.TracerContextMaxFractionDigits)
	require.Zero(t, cfg.TracerContextMaxAccounts)
}
