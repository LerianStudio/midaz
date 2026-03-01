//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultOrchestratorConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultOrchestratorConfig()
	assert.Empty(t, cfg.ToxiproxyAddr, "default ToxiproxyAddr should be empty")
}

func TestDefaultContainerChaosConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerChaosConfig()
	assert.NotZero(t, cfg.StopTimeout, "StopTimeout should have a default")
	assert.NotZero(t, cfg.RestartTimeout, "RestartTimeout should have a default")
}

func TestDefaultNetworkChaosConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultNetworkChaosConfig()
	assert.NotEmpty(t, cfg.Image, "Image should have a default")
	assert.NotZero(t, cfg.MemoryMB, "MemoryMB should have a default")
}

func TestDefaultInfrastructureConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultInfrastructureConfig()
	assert.NotEmpty(t, cfg.NetworkName, "NetworkName should have a default")
	assert.True(t, cfg.SetupToxiproxy, "SetupToxiproxy should default to true")
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	// Verify sentinel errors are defined
	require.Error(t, ErrContainerNotRunning)
	require.Error(t, ErrContainerNotPaused)
	require.Error(t, ErrToxiproxyNotConfigured)
	require.Error(t, ErrRecoveryTimeout)
	require.Error(t, ErrDataIntegrityViolation)
}

func TestAssertNoDataLoss(t *testing.T) {
	t.Parallel()

	// Test with equal values
	AssertNoDataLoss(t, 100, 100, "values should match")

	// Test with comparable struct
	type balance struct {
		amount int64
	}

	before := balance{amount: 1000}
	after := balance{amount: 1000}
	AssertNoDataLoss(t, before, after, "balances should match")
}
