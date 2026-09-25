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

func TestContextDefaultsPreserveExplicitZero(t *testing.T) {
	for _, key := range []string{"CONTEXT_MAX_FRACTION_DIGITS", "CONTEXT_MAX_ACCOUNTS"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	cfg := &Config{}
	applyContextDefaults(cfg)
	require.Equal(t, "128", cfg.ContextMaxFractionDigits)
	require.Equal(t, tracercontract.DefaultResourceProfile().Facts.MaxAccounts, cfg.ContextMaxAccounts)
	require.False(t, cfg.ContextReserveEnabled)
	require.False(t, cfg.ContextPolicyAdminEnabled)
	require.False(t, cfg.ContextLimitAdminEnabled)
	require.Empty(t, cfg.ContextProducerBindings)
	t.Setenv("CONTEXT_MAX_FRACTION_DIGITS", "0")
	t.Setenv("CONTEXT_MAX_ACCOUNTS", "0")
	cfg = &Config{ContextMaxFractionDigits: "0"}
	applyContextDefaults(cfg)
	require.Equal(t, "0", cfg.ContextMaxFractionDigits)
	require.Zero(t, cfg.ContextMaxAccounts)
	_, err := loadContextFactBounds(cfg)
	require.Error(t, err)
}
