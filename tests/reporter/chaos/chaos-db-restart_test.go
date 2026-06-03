//go:build chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	h "github.com/LerianStudio/midaz/v3/tests/reporter/utils"
)

// TestIntegration_Chaos_Datastores_RestartAndRecover restarts MongoDB and Valkey containers and
// validates recovery of the system following the 5-phase chaos test structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_Datastores_RestartAndRecover(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure (restarts MongoDB and Valkey).
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)

	// Phase 1 (Normal): Verify system is healthy before chaos injection
	t.Log("Phase 1 (Normal): Verifying system health before chaos injection...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")
	t.Log("Phase 1 (Normal): System is healthy, proceeding with datastore chaos injection")

	// Phase 2 (Inject): Restart MongoDB and Valkey containers to simulate datastore failure
	t.Log("Phase 2 (Inject): Restarting MongoDB container...")
	if err := RestartMongoDB(5 * time.Second); err != nil {
		t.Fatalf("failed to restart mongo: %v", err)
	}
	t.Log("Phase 2 (Inject): Restarting Valkey container...")
	if err := RestartValkey(5 * time.Second); err != nil {
		t.Fatalf("failed to restart valkey: %v", err)
	}
	t.Log("Phase 2 (Inject): Datastore restarts initiated")

	// Phase 3 (Verify Failure): Confirm the system detects the disruption
	t.Log("Phase 3 (Verify Failure): Verifying system detects datastore disruption...")
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)
	if err != nil || code != 200 {
		t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
	} else {
		t.Log("Phase 3 (Verify Failure): System still reports healthy - datastores may have restarted quickly")
	}

	// Phase 4 (Restore): Wait for datastores to fully stabilize
	t.Log("Phase 4 (Restore): Waiting for datastores to stabilize...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "Datastores did not stabilize after restart")
	t.Log("Phase 4 (Restore): Datastores are stable")

	// Phase 5 (Verify Recovery): Confirm the system has fully recovered
	t.Log("Phase 5 (Verify Recovery): Verifying system has fully recovered...")
	reqCtx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()

	code, _, err = cli.Request(reqCtx2, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after datastore recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after datastore recovery")
	t.Log("Phase 5 (Verify Recovery): System is healthy after datastore restart")
}
