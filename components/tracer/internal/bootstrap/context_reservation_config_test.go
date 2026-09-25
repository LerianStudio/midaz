// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextReservationRequiresNativeIdentityAndBounds(t *testing.T) {
	cfg := &Config{}
	result, err := loadContextReservationConfig(cfg)
	require.NoError(t, err)
	require.Nil(t, result)
	cfg.ContextReserveEnabled = true
	_, err = loadContextReservationConfig(cfg)
	require.Error(t, err)
	cfg.TracerTLSMode = "mtls"
	_, err = loadContextReservationConfig(cfg)
	require.Error(t, err)
	cfg.ContextMaxAccounts = 10
	cfg.ContextMaxEntries = 20
	cfg.ContextMaxTextBytes = 256
	cfg.ContextMaxIntegerDigits = 128
	cfg.ContextMaxFractionDigits = "128"
	cfg.ContextMaxRules = 10
	cfg.ContextMaxExpressionBytes = 5000
	cfg.ContextCELCostLimit = "100000"
	cfg.ContextCELTotalCostLimit = "100000"
	cfg.ContextProducerBindings = `[{"uri":"spiffe://test/ledger","integrationId":"producer","assetNamespace":"official","purposes":["reserve"]}]`
	cfg.ContextLimitMaxScopes = 10
	cfg.ContextLimitMaxScopeBytes = 4096
	cfg.ContextReserveMaxBodyBytes = 65536
	cfg.ContextReserveMaxLimits = 10
	cfg.ContextReserveMaxReservations = 100
	cfg.ContextPolicyCacheEntries = 10
	cfg.ContextPolicyMaxCompilations = 2
	result, err = loadContextReservationConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, cfg.ContextPolicyAdminEnabled, "admission must not implicitly enable administration")
	cfg.ContextReserveMaxReservations = math.MaxInt32 + 1
	_, err = loadContextReservationConfig(cfg)
	require.Error(t, err)
	cfg.ContextReserveMaxReservations = 100
	cfg.TracerTLSMode = "mesh"
	_, err = loadContextReservationConfig(cfg)
	require.Error(t, err)
}
