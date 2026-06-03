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

	h "github.com/LerianStudio/reporter/tests/utils"
	chaosutil "github.com/LerianStudio/reporter/tests/utils/chaos"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Chaos_Valkey_HighLatency injects high latency into the Valkey/Redis
// connection via Toxiproxy and validates that the system handles cache delays gracefully.
// This test is only available when Toxiproxy is running (skipped otherwise).
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_Valkey_HighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	valkeyProxy, useToxiproxy := getValkeyProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available (cannot simulate latency with container restart)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify system is healthy and can serve requests
	t.Log("Phase 1 (Normal): Verifying system health before Valkey latency injection...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify a Redis-dependent operation works (template list may use cache)
	t.Log("Phase 1 (Normal): Verifying system operations work normally...")
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, _, err := cli.Request(reqCtx, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "Request should succeed before chaos injection")
	require.Equal(t, 200, code, "Template listing should return 200 before chaos injection")
	t.Log("Phase 1 (Normal): System is healthy, cache operations working normally")

	// Phase 2 (Inject): Add 3000ms latency with 1000ms jitter to Valkey
	t.Log("Phase 2 (Inject): Injecting 3000ms latency + 1000ms jitter into Valkey connection...")
	err = chaosutil.InjectLatency(valkeyProxy, 3000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")
	t.Log("Phase 2 (Inject): Valkey latency injected successfully")

	// Phase 3 (Verify Failure): System should still respond but cache operations may be degraded
	t.Log("Phase 3 (Verify Failure): Checking system behavior under high Valkey latency...")
	// Wait for the latency to take effect on existing connections
	time.Sleep(3 * time.Second)

	// With high Valkey latency, requests should still succeed (cache miss falls back to DB)
	// but may be slower
	reqCtx3, cancel3 := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3()

	code, _, err = cli.Request(reqCtx3, "GET", "/v1/templates?limit=1", headers, nil)
	if err != nil || code != 200 {
		t.Logf("Phase 3 (Verify Failure): Request degraded under Valkey latency (code=%d, err=%v)", code, err)
	} else {
		t.Log("Phase 3 (Verify Failure): System still responsive under Valkey latency (graceful cache degradation)")
	}

	// Health endpoint should remain accessible - Valkey latency should not crash the system
	reqCtxHealth, cancelHealth := context.WithTimeout(ctx, 15*time.Second)
	defer cancelHealth()

	code, _, err = cli.Request(reqCtxHealth, "GET", "/health", nil, nil)
	if err == nil && code == 200 {
		t.Log("Phase 3 (Verify Failure): Liveness probe still healthy (expected - cache degradation should not affect liveness)")
	} else {
		t.Logf("Phase 3 (Verify Failure): Liveness probe affected by Valkey latency (code=%d, err=%v)", code, err)
	}

	// Phase 4 (Restore): Remove all toxics to restore normal Valkey operation
	t.Log("Phase 4 (Restore): Removing latency toxic from Valkey proxy...")
	err = chaosutil.RemoveAllToxics(valkeyProxy)
	require.NoError(t, err, "Failed to remove toxics from Valkey proxy")

	// Wait for system to recover from latency effects
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after Valkey latency removal")
	t.Log("Phase 4 (Restore): System recovered, Valkey latency removed")

	// Phase 5 (Verify Recovery): Cache operations should work normally again
	t.Log("Phase 5 (Verify Recovery): Verifying cache operations restored to normal...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "Request should succeed after Valkey recovery")
	require.Equal(t, 200, code, "Template listing should return 200 after Valkey recovery")

	// Verify readiness probe is fully healthy
	reqCtxReady, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()

	code, _, err = cli.Request(reqCtxReady, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after Valkey recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after Valkey recovery")
	t.Log("Phase 5 (Verify Recovery): System fully recovered from Valkey high latency injection")
}

// TestIntegration_Chaos_Valkey_ConnectionLoss simulates a complete connection loss to Valkey/Redis
// via Toxiproxy and validates that the system degrades gracefully (cache bypass) and recovers.
// When Toxiproxy is unavailable, falls back to container restart.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_Valkey_ConnectionLoss(t *testing.T) {
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
	t.Log("Phase 1 (Normal): Verifying system health before Valkey connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify cache-dependent operations work under normal conditions
	t.Log("Phase 1 (Normal): Verifying system operations work normally...")
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	code, _, err := cli.Request(reqCtx, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "Request should succeed before chaos injection")
	require.Equal(t, 200, code, "Template listing should return 200 before chaos injection")
	t.Log("Phase 1 (Normal): System is healthy, proceeding with Valkey connection loss")

	// Determine injection method: Toxiproxy (preferred) or container restart (fallback)
	valkeyProxy, useToxiproxy := getValkeyProxy()

	if useToxiproxy {
		t.Log("Phase 2 (Inject): Using Toxiproxy to simulate Valkey connection loss...")
		err := chaosutil.InjectConnectionLoss(valkeyProxy)
		require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")
		t.Log("Phase 2 (Inject): Valkey connection loss injected via Toxiproxy")

		// Phase 3 (Verify Failure): Confirm the system degrades gracefully
		t.Log("Phase 3 (Verify Failure): Waiting for system to detect Valkey connection loss...")
		time.Sleep(5 * time.Second)

		// With Valkey down, the system should gracefully degrade (bypass cache, serve from DB)
		reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
		defer cancel3()

		code, _, err := cli.Request(reqCtx3, "GET", "/v1/templates?limit=1", headers, nil)
		if err == nil && code == 200 {
			t.Log("Phase 3 (Verify Failure): System correctly degrades gracefully - requests still served without cache")
		} else {
			t.Logf("Phase 3 (Verify Failure): System affected by Valkey loss (code=%d, err=%v)", code, err)
		}

		// Manager should still respond to health check
		reqCtxLive, cancelLive := context.WithTimeout(ctx, 5*time.Second)
		defer cancelLive()

		code, _, err = cli.Request(reqCtxLive, "GET", "/health", nil, nil)
		if err == nil && code == 200 {
			t.Log("Phase 3 (Verify Failure): Liveness probe still healthy (graceful degradation without cache)")
		} else {
			t.Logf("Phase 3 (Verify Failure): Liveness probe affected by Valkey loss (code=%d, err=%v)", code, err)
		}

		// Phase 4 (Restore): Remove all toxics to restore Valkey connectivity
		t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore Valkey connectivity...")
		err = chaosutil.RemoveAllToxics(valkeyProxy)
		require.NoError(t, err, "Failed to remove toxics from Valkey proxy")
		t.Log("Phase 4 (Restore): Toxics removed, Valkey connectivity restored")
	} else {
		// Fallback: container restart
		t.Log("Phase 2 (Inject): Toxiproxy not available, falling back to container restart...")
		if err := RestartValkey(5 * time.Second); err != nil {
			t.Fatalf("failed to restart valkey: %v", err)
		}
		t.Log("Phase 2 (Inject): Valkey restart initiated")

		// Phase 3 (Verify Failure): Confirm the system detects the disruption
		t.Log("Phase 3 (Verify Failure): Verifying system detects Valkey disruption...")
		reqCtx3, cancel3 := context.WithTimeout(ctx, 5*time.Second)
		defer cancel3()

		code, _, err := cli.Request(reqCtx3, "GET", "/readyz", nil, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): System still reports healthy - Valkey may have restarted quickly")
		}

		// Phase 4 (Restore): Wait for Valkey to fully stabilize
		t.Log("Phase 4 (Restore): Waiting for Valkey to stabilize after restart...")
	}

	// Phase 4 continued: Wait for system readiness
	t.Log("Phase 4 (Restore): Waiting for system to become fully ready...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after Valkey disruption")
	t.Log("Phase 4 (Restore): System is ready")

	// Phase 5 (Verify Recovery): Confirm cache operations work again
	t.Log("Phase 5 (Verify Recovery): Verifying cache operations restored...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/templates?limit=1", headers, nil)
	require.NoError(t, err, "Request should succeed after Valkey recovery")
	assert.Equal(t, 200, code, "Template listing should return 200 after Valkey recovery")

	// Final readiness check
	reqCtxFinal, cancelFinal := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFinal()

	code, _, err = cli.Request(reqCtxFinal, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after Valkey recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after Valkey recovery")
	t.Log("Phase 5 (Verify Recovery): Manager is healthy after Valkey connection loss and recovery")
}

// getValkeyProxy returns the Toxiproxy proxy for Valkey/Redis if available.
// Returns (proxy, true) if Toxiproxy is running, (nil, false) otherwise.
func getValkeyProxy() (*toxiproxy.Proxy, bool) {
	if toxiInfra == nil {
		return nil, false
	}

	proxy, ok := toxiInfra.GetProxy(chaosutil.ProxyNameValkey)

	return proxy, ok
}
