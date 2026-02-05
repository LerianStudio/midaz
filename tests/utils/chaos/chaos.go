// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration || chaos

package chaos

import (
	"context"
	"testing"
	"time"

	toxiproxyclient "github.com/Shopify/toxiproxy/v2/client"
	docker "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

// Orchestrator coordinates chaos injection operations.
// It provides a unified interface for container lifecycle and network chaos.
type Orchestrator struct {
	docker    *docker.Client
	toxiproxy *toxiproxyclient.Client
	t         *testing.T
}

// OrchestratorConfig holds configuration for the chaos orchestrator.
type OrchestratorConfig struct {
	// ToxiproxyAddr is the address of the Toxiproxy server (e.g., "localhost:8474").
	// If empty, network chaos operations will not be available.
	ToxiproxyAddr string
}

// DefaultOrchestratorConfig returns the default orchestrator configuration.
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		ToxiproxyAddr: "", // Will be set from infrastructure
	}
}

// NewOrchestrator creates a new chaos orchestrator.
// It initializes Docker client for container operations.
func NewOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	return NewOrchestratorWithConfig(t, DefaultOrchestratorConfig())
}

// NewOrchestratorWithConfig creates a new chaos orchestrator with custom configuration.
func NewOrchestratorWithConfig(t *testing.T, cfg OrchestratorConfig) *Orchestrator {
	t.Helper()

	ctx := context.Background()
	_ = ctx // Silence unused variable warning

	// Initialize Docker client
	dockerCli, err := docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	require.NoError(t, err, "failed to create Docker client")

	orch := &Orchestrator{
		docker: dockerCli,
		t:      t,
	}

	// Initialize Toxiproxy client if address provided
	if cfg.ToxiproxyAddr != "" {
		orch.toxiproxy = toxiproxyclient.NewClient(cfg.ToxiproxyAddr)
	}

	return orch
}

// SetToxiproxyClient sets the Toxiproxy client for network chaos operations.
// This is typically called by Infrastructure.SetupToxiproxy().
func (o *Orchestrator) SetToxiproxyClient(c *toxiproxyclient.Client) {
	o.toxiproxy = c
}

// Close releases resources held by the orchestrator.
func (o *Orchestrator) Close() error {
	if o.docker != nil {
		return o.docker.Close()
	}

	return nil
}

// WaitForRecovery polls a check function until it succeeds or times out.
// This is useful for waiting for a service to recover after chaos injection.
func (o *Orchestrator) WaitForRecovery(ctx context.Context, check func() error, timeout time.Duration) error {
	o.t.Helper()

	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := check(); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
	}

	if lastErr != nil {
		return lastErr
	}

	return context.DeadlineExceeded
}

// RunWithChaos executes a function while chaos is injected, then cleans up.
// This is a convenience wrapper for inject-test-cleanup patterns.
func (o *Orchestrator) RunWithChaos(
	ctx context.Context,
	inject func(ctx context.Context) error,
	cleanup func(ctx context.Context) error,
	test func(ctx context.Context) error,
) error {
	o.t.Helper()

	// Inject chaos
	if err := inject(ctx); err != nil {
		return err
	}

	// Ensure cleanup runs even if test fails
	defer func() {
		if cleanupErr := cleanup(ctx); cleanupErr != nil {
			o.t.Logf("chaos cleanup failed: %v", cleanupErr)
		}
	}()

	// Run test
	return test(ctx)
}
