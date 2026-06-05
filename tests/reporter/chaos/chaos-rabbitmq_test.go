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

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
	chaosutil "github.com/LerianStudio/midaz/v4/tests/reporter/utils/chaos"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Chaos_RabbitMQ_RestartAndRecover restarts the RabbitMQ container and validates
// recovery of the system following the 5-phase chaos test structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
//
// When Toxiproxy is available, uses bandwidth=0 toxic to simulate connection loss.
// Falls back to container restart when Toxiproxy is not available.
func TestIntegration_Chaos_RabbitMQ_RestartAndRecover(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
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
	t.Log("Phase 1 (Normal): System is healthy, proceeding with chaos injection")

	// Determine injection method: Toxiproxy (preferred) or container restart (fallback)
	rabbitProxy, useToxiproxy := getRabbitMQProxy()

	if useToxiproxy {
		t.Log("Phase 2 (Inject): Using Toxiproxy to simulate RabbitMQ connection loss...")
		err := chaosutil.InjectConnectionLoss(rabbitProxy)
		require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")
		t.Log("Phase 2 (Inject): RabbitMQ connection loss injected via Toxiproxy")

		// Phase 3 (Verify Failure): Confirm the system detects the disruption
		t.Log("Phase 3 (Verify Failure): Waiting for system to detect connection loss...")
		// Give the Manager time to detect the broken connection
		time.Sleep(5 * time.Second)

		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): System still reports healthy - connection loss may not yet be detected")
		}

		// Phase 4 (Restore): Remove all toxics to restore normal operation
		t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore RabbitMQ connectivity...")
		err = chaosutil.RemoveAllToxics(rabbitProxy)
		require.NoError(t, err, "Failed to remove toxics from RabbitMQ proxy")
		t.Log("Phase 4 (Restore): Toxics removed, RabbitMQ connectivity restored")
	} else {
		// Fallback: container restart
		t.Log("Phase 2 (Inject): Toxiproxy not available, falling back to container restart...")
		if err := RestartRabbitMQ(5 * time.Second); err != nil {
			t.Fatalf("failed to restart rabbitmq: %v", err)
		}
		t.Log("Phase 2 (Inject): RabbitMQ restart initiated")

		// Phase 3 (Verify Failure): Confirm the system detects the disruption
		t.Log("Phase 3 (Verify Failure): Verifying system detects RabbitMQ disruption...")
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): System still reports healthy - RabbitMQ may have restarted quickly")
		}

		// Phase 4 (Restore): Wait for RabbitMQ to fully stabilize
		t.Log("Phase 4 (Restore): Waiting for RabbitMQ to stabilize after restart...")
	}

	// Phase 4 continued: Wait for system readiness
	t.Log("Phase 4 (Restore): Waiting for system to become fully ready...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after RabbitMQ disruption")
	t.Log("Phase 4 (Restore): System is ready")

	// Phase 5 (Verify Recovery): Confirm the system has fully recovered
	t.Log("Phase 5 (Verify Recovery): Verifying Manager has fully recovered...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, _, err := cli.Request(reqCtx5, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after recovery")
	t.Log("Phase 5 (Verify Recovery): Manager is healthy after RabbitMQ disruption and recovery")
}

// TestIntegration_Chaos_RabbitMQ_HighLatency injects high latency into the RabbitMQ
// connection via Toxiproxy and validates that the system handles it gracefully.
// This test is only available when Toxiproxy is running (skipped otherwise).
func TestIntegration_Chaos_RabbitMQ_HighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	rabbitProxy, useToxiproxy := getRabbitMQProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available (cannot simulate latency with container restart)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)

	// Phase 1 (Normal): Verify system is healthy
	t.Log("Phase 1 (Normal): Verifying system health...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Phase 2 (Inject): Add 5000ms latency with 2000ms jitter to RabbitMQ
	t.Log("Phase 2 (Inject): Injecting 5000ms latency + 2000ms jitter into RabbitMQ connection...")
	err := chaosutil.InjectLatency(rabbitProxy, 5000, 2000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")

	// Phase 3 (Verify Failure): System should still respond but may be degraded
	t.Log("Phase 3 (Verify Failure): Checking system behavior under high latency...")
	// Wait for the latency to take effect on existing connections
	time.Sleep(3 * time.Second)

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)
	if err != nil || code != 200 {
		t.Logf("Phase 3 (Verify Failure): System degraded under latency (code=%d, err=%v)", code, err)
	} else {
		t.Log("Phase 3 (Verify Failure): System still responsive under latency (graceful handling)")
	}

	// Phase 4 (Restore): Remove all toxics
	t.Log("Phase 4 (Restore): Removing latency toxic...")
	err = chaosutil.RemoveAllToxics(rabbitProxy)
	require.NoError(t, err, "Failed to remove toxics from RabbitMQ proxy")

	// Wait for system to recover from latency effects
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System did not recover after latency removal")

	// Phase 5 (Verify Recovery): System should be fully healthy
	t.Log("Phase 5 (Verify Recovery): Verifying full system recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, _, err = cli.Request(reqCtx5, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after latency recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after latency recovery")
	t.Log("Phase 5 (Verify Recovery): System fully recovered from high latency injection")
}

// getRabbitMQProxy returns the Toxiproxy proxy for RabbitMQ if available.
// Returns (proxy, true) if Toxiproxy is running, (nil, false) otherwise.
func getRabbitMQProxy() (*toxiproxy.Proxy, bool) {
	if toxiInfra == nil {
		return nil, false
	}

	proxy, ok := toxiInfra.GetProxy(chaosutil.ProxyNameRabbitMQ)

	return proxy, ok
}
