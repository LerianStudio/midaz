//go:build chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils"
	chaosutil "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils/chaos"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Chaos_SeaweedFS_ConnectionLoss simulates a complete connection loss to SeaweedFS
// (S3 storage) via Toxiproxy and validates that the system handles storage errors gracefully.
// When Toxiproxy is unavailable, falls back to container restart.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_SeaweedFS_ConnectionLoss(t *testing.T) {
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
	t.Log("Phase 1 (Normal): Verifying system health before SeaweedFS connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify system can accept report creation requests (triggers storage usage in worker)
	t.Log("Phase 1 (Normal): Verifying system operations work normally...")
	templateID, ok := getAnyTemplateIDWithRetry(ctx, t, cli, headers, 10, 2*time.Second)
	if !ok {
		t.Skip("No templates available or service unstable for chaos testing")
	}
	t.Logf("Phase 1 (Normal): System is healthy, using template: %s", templateID)

	// Determine injection method: Toxiproxy (preferred) or container restart (fallback)
	seaweedProxy, useToxiproxy := getSeaweedFSProxy()

	if useToxiproxy {
		t.Log("Phase 2 (Inject): Using Toxiproxy to simulate SeaweedFS connection loss...")
		err := chaosutil.InjectConnectionLoss(seaweedProxy)
		require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")
		t.Log("Phase 2 (Inject): SeaweedFS connection loss injected via Toxiproxy")

		// Phase 3 (Verify Failure): Confirm the system handles storage errors
		t.Log("Phase 3 (Verify Failure): Waiting for system to detect SeaweedFS connection loss...")
		time.Sleep(5 * time.Second)

		// Create a report to exercise the storage path - report creation should still succeed
		// at the API level (report created in MongoDB, queued to RabbitMQ), but the worker
		// will fail when trying to store the generated file in S3
		t.Log("Phase 3 (Verify Failure): Creating report during SeaweedFS outage...")
		payload := map[string]any{
			"templateId": templateID,
			"filters":    map[string]any{},
		}

		reqCtx3, cancel3 := context.WithTimeout(ctx, 15*time.Second)
		defer cancel3()

		code, body, err := cli.Request(reqCtx3, "POST", "/v1/reports", headers, payload)
		if err != nil {
			t.Logf("Phase 3 (Verify Failure): Report creation request failed (code=%d, err=%v)", code, err)
		} else if code == 201 {
			var reportResp struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(body, &reportResp) == nil {
				t.Logf("Phase 3 (Verify Failure): Report created (id=%s) - worker will encounter S3 errors during generation", reportResp.ID)
			}
		} else {
			t.Logf("Phase 3 (Verify Failure): Report creation returned code=%d during SeaweedFS outage", code)
		}

		// Manager should still respond to non-storage operations
		reqCtxHealth, cancelHealth := context.WithTimeout(ctx, 10*time.Second)
		defer cancelHealth()

		code, _, err = cli.Request(reqCtxHealth, "GET", "/v1/templates?limit=1", headers, nil)
		if err == nil && code == 200 {
			t.Log("Phase 3 (Verify Failure): Non-storage operations still work (graceful degradation)")
		} else {
			t.Logf("Phase 3 (Verify Failure): Non-storage operations also affected (code=%d, err=%v)", code, err)
		}

		// Phase 4 (Restore): Remove all toxics to restore SeaweedFS connectivity
		t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore SeaweedFS connectivity...")
		err = chaosutil.RemoveAllToxics(seaweedProxy)
		require.NoError(t, err, "Failed to remove toxics from SeaweedFS proxy")
		t.Log("Phase 4 (Restore): Toxics removed, SeaweedFS connectivity restored")
	} else {
		// Fallback: container restart
		t.Log("Phase 2 (Inject): Toxiproxy not available, falling back to container restart...")
		if SeaweedContainer == nil {
			t.Skip("SeaweedFS container not available for chaos testing")
		}

		if err := SeaweedContainer.Restart(ctx, 5*time.Second); err != nil {
			t.Fatalf("failed to restart seaweedfs: %v", err)
		}
		t.Log("Phase 2 (Inject): SeaweedFS restart initiated")

		// Phase 3 (Verify Failure): Confirm the system detects the disruption
		t.Log("Phase 3 (Verify Failure): Verifying system detects SeaweedFS disruption...")
		reqCtx3, cancel3 := context.WithTimeout(ctx, 5*time.Second)
		defer cancel3()

		code, _, err := cli.Request(reqCtx3, "GET", "/readyz", nil, nil)
		if err != nil || code != 200 {
			t.Logf("Phase 3 (Verify Failure): System correctly reports degraded state (code=%d, err=%v)", code, err)
		} else {
			t.Log("Phase 3 (Verify Failure): System still reports healthy - SeaweedFS may have restarted quickly")
		}

		// Phase 4 (Restore): Wait for SeaweedFS to fully stabilize
		t.Log("Phase 4 (Restore): Waiting for SeaweedFS to stabilize after restart...")
	}

	// Phase 4 continued: Wait for system readiness
	t.Log("Phase 4 (Restore): Waiting for system to become fully ready...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after SeaweedFS disruption")
	t.Log("Phase 4 (Restore): System is ready")

	// Phase 5 (Verify Recovery): Confirm storage operations work again
	t.Log("Phase 5 (Verify Recovery): Verifying storage operations restored...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	// Verify the system can accept new report requests (which will exercise storage on the worker)
	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	code, _, err := cli.Request(reqCtx5, "POST", "/v1/reports", headers, payload)
	require.NoError(t, err, "Report creation should succeed after SeaweedFS recovery")
	assert.True(t, code == 200 || code == 201,
		"Expected success status after SeaweedFS recovery, got code=%d", code)

	// Final readiness check
	reqCtxFinal, cancelFinal := context.WithTimeout(ctx, 10*time.Second)
	defer cancelFinal()

	code, _, err = cli.Request(reqCtxFinal, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after SeaweedFS recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after SeaweedFS recovery")
	t.Log("Phase 5 (Verify Recovery): Manager is healthy after SeaweedFS connection loss and recovery")
}

// TestIntegration_Chaos_SeaweedFS_HighLatency injects high latency into the SeaweedFS
// S3 connection via Toxiproxy and validates that the system handles storage timeouts gracefully.
// This test is only available when Toxiproxy is running (skipped otherwise).
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Verify Recovery)
func TestIntegration_Chaos_SeaweedFS_HighLatency(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	seaweedProxy, useToxiproxy := getSeaweedFSProxy()
	if !useToxiproxy {
		t.Skip("Skipping latency test - Toxiproxy not available (cannot simulate latency with container restart)")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// Phase 1 (Normal): Verify system is healthy and can serve requests
	t.Log("Phase 1 (Normal): Verifying system health before SeaweedFS latency injection...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify system can handle storage-dependent operations
	t.Log("Phase 1 (Normal): Verifying system operations work normally...")
	templateID, ok := getAnyTemplateIDWithRetry(ctx, t, cli, headers, 10, 2*time.Second)
	if !ok {
		t.Skip("No templates available or service unstable for chaos testing")
	}
	t.Logf("Phase 1 (Normal): System is healthy, using template: %s", templateID)

	// Phase 2 (Inject): Add 5000ms latency with 2000ms jitter to SeaweedFS S3
	t.Log("Phase 2 (Inject): Injecting 5000ms latency + 2000ms jitter into SeaweedFS S3 connection...")
	err := chaosutil.InjectLatency(seaweedProxy, 5000, 2000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")
	t.Log("Phase 2 (Inject): SeaweedFS S3 latency injected successfully")

	// Phase 3 (Verify Failure): System should still accept requests but storage may timeout
	t.Log("Phase 3 (Verify Failure): Checking system behavior under high S3 latency...")
	// Wait for the latency to take effect
	time.Sleep(3 * time.Second)

	// API operations that do not touch S3 should still work normally
	reqCtx3, cancel3 := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3()

	code, _, err := cli.Request(reqCtx3, "GET", "/v1/templates?limit=1", headers, nil)
	if err == nil && code == 200 {
		t.Log("Phase 3 (Verify Failure): Non-storage API operations still responsive (expected)")
	} else {
		t.Logf("Phase 3 (Verify Failure): Non-storage operations affected (code=%d, err=%v)", code, err)
	}

	// Create a report to trigger S3 usage in the worker pipeline
	t.Log("Phase 3 (Verify Failure): Creating report to exercise storage path under latency...")
	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	reqCtxReport, cancelReport := context.WithTimeout(ctx, 15*time.Second)
	defer cancelReport()

	code, body, err := cli.Request(reqCtxReport, "POST", "/v1/reports", headers, payload)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): Report creation failed under S3 latency (code=%d, err=%v)", code, err)
	} else if code == 201 {
		var reportResp struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &reportResp) == nil {
			t.Logf("Phase 3 (Verify Failure): Report created (id=%s) - worker will experience S3 latency during file upload", reportResp.ID)
		}
	} else {
		t.Logf("Phase 3 (Verify Failure): Report creation returned code=%d under S3 latency", code)
	}

	// Health endpoint should remain accessible
	reqCtxHealth, cancelHealth := context.WithTimeout(ctx, 15*time.Second)
	defer cancelHealth()

	code, _, err = cli.Request(reqCtxHealth, "GET", "/health", nil, nil)
	if err == nil && code == 200 {
		t.Log("Phase 3 (Verify Failure): Liveness probe still healthy (S3 latency should not affect liveness)")
	} else {
		t.Logf("Phase 3 (Verify Failure): Liveness probe affected by S3 latency (code=%d, err=%v)", code, err)
	}

	// Phase 4 (Restore): Remove all toxics to restore normal S3 operation
	t.Log("Phase 4 (Restore): Removing latency toxic from SeaweedFS proxy...")
	err = chaosutil.RemoveAllToxics(seaweedProxy)
	require.NoError(t, err, "Failed to remove toxics from SeaweedFS proxy")

	// Wait for system to recover
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after SeaweedFS latency removal")
	t.Log("Phase 4 (Restore): System recovered, SeaweedFS latency removed")

	// Phase 5 (Verify Recovery): Storage operations should work normally again
	t.Log("Phase 5 (Verify Recovery): Verifying storage operations restored to normal...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	// Verify report creation (exercises full pipeline including S3)
	recoveryPayload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	code, _, err = cli.Request(reqCtx5, "POST", "/v1/reports", headers, recoveryPayload)
	require.NoError(t, err, "Report creation should succeed after SeaweedFS recovery")
	assert.True(t, code == 200 || code == 201,
		"Expected success status after SeaweedFS recovery, got code=%d", code)

	// Verify readiness probe is fully healthy
	reqCtxReady, cancelReady := context.WithTimeout(ctx, 10*time.Second)
	defer cancelReady()

	code, _, err = cli.Request(reqCtxReady, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Manager should respond without error after SeaweedFS recovery")
	require.Equal(t, 200, code, "Manager readiness probe should return 200 after SeaweedFS recovery")
	t.Log("Phase 5 (Verify Recovery): System fully recovered from SeaweedFS high latency injection")
}

// getSeaweedFSProxy returns the Toxiproxy proxy for SeaweedFS if available.
// Returns (proxy, true) if Toxiproxy is running, (nil, false) otherwise.
func getSeaweedFSProxy() (*toxiproxy.Proxy, bool) {
	if toxiInfra == nil {
		return nil, false
	}

	proxy, ok := toxiInfra.GetProxy(chaosutil.ProxyNameSeaweedFS)

	return proxy, ok
}
