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

	h "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils"
	chaosutil "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils/chaos"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Chaos_MongoDB_HighLatency injects high latency into the MongoDB
// connection via Toxiproxy and validates that the system degrades gracefully without crashing.
// This test is only available when Toxiproxy is running (skipped otherwise).
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_MongoDB_HighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available (cannot simulate latency with container restart)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify system is healthy and can serve requests
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB latency injection...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify MongoDB-dependent operations work under normal conditions
	t.Log("Phase 1 (Normal): Verifying MongoDB-dependent operations work normally...")
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, _, err := cli.Request(reqCtx, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "MongoDB-dependent request should succeed before chaos injection")
	require.Equal(t, 200, code, "Template listing should return 200 before chaos injection")
	t.Log("Phase 1 (Normal): System is healthy, MongoDB operations working normally")

	// Phase 2 (Inject): Add 3000ms latency with 1000ms jitter to MongoDB
	t.Log("Phase 2 (Inject): Injecting 3000ms latency + 1000ms jitter into MongoDB connection...")
	err = chaosutil.InjectLatency(mongoProxy, 3000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")
	t.Log("Phase 2 (Inject): MongoDB latency injected successfully")

	// Phase 3 (Verify Failure): System should still respond but may be degraded
	t.Log("Phase 3 (Verify Failure): Checking system behavior under high MongoDB latency...")
	// Wait for the latency to take effect on existing connections
	time.Sleep(3 * time.Second)

	// With high MongoDB latency, template listing should be slow or fail
	reqCtx3, cancel3 := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3()

	code, _, err = cli.Request(reqCtx3, "GET", "/v1/templates?limit=1", headers, nil)
	if err != nil || code != 200 {
		t.Logf("Phase 3 (Verify Failure): MongoDB-dependent request degraded (code=%d, err=%v)", code, err)
	} else {
		t.Log("Phase 3 (Verify Failure): System still responsive under latency (graceful handling with timeout)")
	}

	// Health endpoint should still be accessible even with MongoDB latency
	reqCtxHealth, cancelHealth := context.WithTimeout(ctx, 15*time.Second)
	defer cancelHealth()

	code, _, err = cli.Request(reqCtxHealth, "GET", "/health", nil, nil)
	if err == nil && code == 200 {
		t.Log("Phase 3 (Verify Failure): Liveness probe still healthy (expected - liveness should not depend on MongoDB latency)")
	} else {
		t.Logf("Phase 3 (Verify Failure): Liveness probe affected by latency (code=%d, err=%v)", code, err)
	}

	// Phase 4 (Restore): Remove all toxics to restore normal MongoDB operation
	t.Log("Phase 4 (Restore): Removing latency toxic from MongoDB proxy...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	// Wait for system to recover from latency effects
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB latency removal")
	t.Log("Phase 4 (Restore): System recovered, MongoDB latency removed")

	// Phase 5 (Verify Recovery): MongoDB-dependent operations should work normally again
	t.Log("Phase 5 (Verify Recovery): Verifying MongoDB operations restored to normal...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "MongoDB-dependent request should succeed after recovery")
	require.Equal(t, 200, code, "Template listing should return 200 after recovery")

	// Verify readiness probe is fully healthy
	reqCtxReady, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()

	code, _, err = cli.Request(reqCtxReady, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after MongoDB recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after MongoDB recovery")
	t.Log("Phase 5 (Verify Recovery): System fully recovered from MongoDB high latency injection")
}

// TestIntegration_Chaos_MongoDB_ConnectionLoss simulates a complete connection loss to MongoDB
// via Toxiproxy and validates that the system handles errors gracefully and recovers.
// When Toxiproxy is unavailable, falls back to container restart.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_MongoDB_ConnectionLoss(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify system is healthy before chaos injection
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify MongoDB-dependent operations work under normal conditions
	t.Log("Phase 1 (Normal): Verifying MongoDB-dependent operations work normally...")
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, _, err := cli.Request(reqCtx, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "MongoDB-dependent request should succeed before chaos injection")
	require.Equal(t, 200, code, "Template listing should return 200 before chaos injection")
	t.Log("Phase 1 (Normal): System is healthy, proceeding with MongoDB connection loss")

	// Determine injection method: Toxiproxy (preferred) or container restart (fallback)
	mongoProxy, useToxiproxy := getMongoDBProxy()

	if useToxiproxy {
		t.Log("Phase 2 (Inject): Using Toxiproxy to simulate MongoDB connection loss...")
		err := chaosutil.InjectConnectionLoss(mongoProxy)
		require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")
		t.Log("Phase 2 (Inject): MongoDB connection loss injected via Toxiproxy")

		// Phase 3 (Verify Failure): Confirm the system detects the disruption
		t.Log("Phase 3 (Verify Failure): Waiting for system to detect MongoDB connection loss...")
		time.Sleep(5 * time.Second)

		// MongoDB-dependent operations should fail or timeout
		reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
		defer cancel3()

		code, _, err := cli.Request(reqCtx3, "GET", "/v1/templates?limit=1", headers, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): MongoDB-dependent request correctly failed (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): Request still succeeded - cached result or connection loss not yet detected")
		}

		// Manager should still respond to health check (liveness should not depend on MongoDB)
		reqCtxLive, cancelLive := context.WithTimeout(ctx, 5*time.Second)
		defer cancelLive()

		code, _, err = cli.Request(reqCtxLive, "GET", "/health", nil, nil)
		if err == nil && code == 200 {
			t.Log("Phase 3 (Verify Failure): Liveness probe still healthy (graceful degradation)")
		} else {
			t.Logf("Phase 3 (Verify Failure): Liveness probe also affected (code=%d, err=%v)", code, err)
		}

		// Readiness probe should report unhealthy when MongoDB is down
		reqCtxReady, cancelReady := context.WithTimeout(ctx, 5*time.Second)
		defer cancelReady()

		code, _, err = cli.Request(reqCtxReady, "GET", "/readyz", nil, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): System still reports healthy - MongoDB loss may not yet be detected")
		}

		// Phase 4 (Restore): Remove all toxics to restore MongoDB connectivity
		t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore MongoDB connectivity...")
		err = chaosutil.RemoveAllToxics(mongoProxy)
		require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")
		t.Log("Phase 4 (Restore): Toxics removed, MongoDB connectivity restored")
	} else {
		// Fallback: container restart
		t.Log("Phase 2 (Inject): Toxiproxy not available, falling back to container restart...")
		if err := RestartMongoDB(5 * time.Second); err != nil {
			t.Fatalf("failed to restart mongodb: %v", err)
		}
		t.Log("Phase 2 (Inject): MongoDB restart initiated")

		// Phase 3 (Verify Failure): Confirm the system detects the disruption
		t.Log("Phase 3 (Verify Failure): Verifying system detects MongoDB disruption...")
		reqCtx3, cancel3 := context.WithTimeout(ctx, 5*time.Second)
		defer cancel3()

		code, _, err := cli.Request(reqCtx3, "GET", "/readyz", nil, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): System still reports healthy - MongoDB may have restarted quickly")
		}

		// Phase 4 (Restore): Wait for MongoDB to fully stabilize
		t.Log("Phase 4 (Restore): Waiting for MongoDB to stabilize after restart...")
	}

	// Phase 4 continued: Wait for system readiness
	t.Log("Phase 4 (Restore): Waiting for system to become fully ready...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB disruption")
	t.Log("Phase 4 (Restore): System is ready")

	// Phase 5 (Verify Recovery): Confirm MongoDB-dependent operations work again
	t.Log("Phase 5 (Verify Recovery): Verifying MongoDB-dependent operations restored...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "MongoDB-dependent request should succeed after recovery")
	assert.Equal(t, 200, code, "Template listing should return 200 after MongoDB recovery")

	// Final readiness check
	reqCtxFinal, cancelFinal := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFinal()

	code, _, err = cli.Request(reqCtxFinal, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after MongoDB recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after MongoDB recovery")
	t.Log("Phase 5 (Verify Recovery): Manager is healthy after MongoDB connection loss and recovery")
}

// getMongoDBProxy returns the Toxiproxy proxy for MongoDB if available.
// Returns (proxy, true) if Toxiproxy is running, (nil, false) otherwise.
func getMongoDBProxy() (*toxiproxy.Proxy, bool) {
	if toxiInfra == nil {
		return nil, false
	}

	proxy, ok := toxiInfra.GetProxy(chaosutil.ProxyNameMongoDB)

	return proxy, ok
}
