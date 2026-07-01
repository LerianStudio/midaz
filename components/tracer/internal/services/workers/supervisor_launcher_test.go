// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerSupervisor_Run_ReturnsOnShutdown verifies that Run blocks until
// Shutdown is called from another goroutine, then returns nil.
// This satisfies the libCommons.App contract so the supervisor can be
// registered as a Launcher app in multi-tenant mode.
func TestWorkerSupervisor_Run_ReturnsOnShutdown(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)

	launcher := libCommons.NewLauncher()
	done := make(chan error, 1)

	go func() {
		done <- sup.Run(launcher)
	}()

	// Shutdown closes shutdownCh; Run observes the closed channel and returns.
	// No sleep needed: close-then-receive is a valid happens-before relationship
	// whether Run has reached the <-shutdownCh or not yet.
	sup.Shutdown()

	select {
	case err := <-done:
		assert.NoError(t, err, "Run should return nil on clean shutdown")
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor.Run did not return within 2s of Shutdown")
	}
}

// TestWorkerSupervisor_Run_CleansUpTenantWorkers verifies that Run's exit
// path (triggered by Shutdown) cleanly tears down every per-tenant worker set.
// This is the Deliverable D "shutdown coordination" assertion for N=3 tenants.
func TestWorkerSupervisor_Run_CleansUpTenantWorkers(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)

	for _, id := range []string{"t-1", "t-2", "t-3"} {
		require.NoError(t, sup.EnsureWorkers(t.Context(), id))
	}

	require.Equal(t, 3, sup.tenantCount())

	launcher := libCommons.NewLauncher()
	done := make(chan error, 1)

	go func() {
		done <- sup.Run(launcher)
	}()

	start := time.Now()
	sup.Shutdown()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor.Run did not return within 2s of Shutdown with 3 tenants")
	}

	assert.Less(t, time.Since(start), 2*time.Second,
		"Shutdown should complete within 2s even with multiple tenants")
	assert.Equal(t, 0, sup.tenantCount(), "all tenant worker sets must be torn down")
}

// TestWorkerSupervisor_Run_ZeroTenants verifies clean shutdown when no tenants
// have been registered. This is the 0-tenant leg of Deliverable D.
func TestWorkerSupervisor_Run_ZeroTenants(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)

	launcher := libCommons.NewLauncher()
	done := make(chan error, 1)

	go func() {
		done <- sup.Run(launcher)
	}()

	sup.Shutdown()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor.Run did not return within 2s when zero tenants registered")
	}
}
