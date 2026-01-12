//go:build integration || chaos

package chaos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultOrchestratorConfig(t *testing.T) {
	cfg := DefaultOrchestratorConfig()
	assert.Empty(t, cfg.ToxiproxyAddr, "default ToxiproxyAddr should be empty")
}

func TestDefaultContainerChaosConfig(t *testing.T) {
	cfg := DefaultContainerChaosConfig()
	assert.NotZero(t, cfg.StopTimeout, "StopTimeout should have a default")
	assert.NotZero(t, cfg.RestartTimeout, "RestartTimeout should have a default")
}

func TestDefaultNetworkChaosConfig(t *testing.T) {
	cfg := DefaultNetworkChaosConfig()
	assert.NotEmpty(t, cfg.Image, "Image should have a default")
	assert.NotZero(t, cfg.MemoryMB, "MemoryMB should have a default")
}

func TestDefaultInfrastructureConfig(t *testing.T) {
	cfg := DefaultInfrastructureConfig()
	assert.NotEmpty(t, cfg.NetworkName, "NetworkName should have a default")
	assert.True(t, cfg.SetupToxiproxy, "SetupToxiproxy should default to true")
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors are defined
	assert.NotNil(t, ErrContainerNotRunning)
	assert.NotNil(t, ErrContainerNotPaused)
	assert.NotNil(t, ErrToxiproxyNotConfigured)
	assert.NotNil(t, ErrRecoveryTimeout)
	assert.NotNil(t, ErrDataIntegrityViolation)
}

func TestAssertNoDataLoss(t *testing.T) {
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

