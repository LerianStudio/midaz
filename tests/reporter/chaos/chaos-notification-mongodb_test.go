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

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
	chaosutil "github.com/LerianStudio/midaz/v4/tests/reporter/utils/chaos"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notificationEndpointResponse mirrors the JSON response from GET /v1/deadlines/notifications
// for chaos test assertions.
type notificationEndpointResponse struct {
	Items []notificationEndpointItem `json:"items"`
	Total int                        `json:"total"`
}

// notificationEndpointItem mirrors a single notification item in the response.
type notificationEndpointItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Type             string `json:"type"`
	DueDate          string `json:"dueDate"`
	Frequency        string `json:"frequency"`
	Color            string `json:"color"`
	Severity         string `json:"severity"`
	DaysUntilDue     int    `json:"daysUntilDue"`
	NotifyDaysBefore int    `json:"notifyDaysBefore"`
}

// --- CS-1: MongoDB Connection Loss during notification query ---

// TestIntegration_Chaos_Notification_MongoDB_ConnectionLoss simulates a complete MongoDB
// connection loss via Toxiproxy and validates that the notifications endpoint
// (GET /v1/deadlines/notifications) returns an error gracefully (no panic/crash),
// then verifies full recovery after connectivity is restored.
//
// The notification handler calls FindActiveNotifiable which queries MongoDB.
// Connection loss must be handled without crashing the process.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestIntegration_Chaos_Notification_MongoDB_ConnectionLoss(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping connection loss test - Toxiproxy not available")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// -- Phase 1 (Normal): Verify system is healthy and notifications endpoint works --
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify notifications endpoint returns valid data under normal conditions
	t.Log("Phase 1 (Normal): Verifying notifications endpoint returns valid data...")
	reqCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()

	code, body, err := cli.Request(reqCtx1, "GET", "/v1/deadlines/notifications", headers, nil)
	require.NoError(t, err, "Notifications request should succeed before chaos injection")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications should return 200 before chaos injection")

	var baselineResp notificationEndpointResponse
	require.NoError(t, json.Unmarshal(body, &baselineResp), "Should unmarshal notifications response")
	t.Logf("Phase 1 (Normal): Baseline notifications - total=%d", baselineResp.Total)

	// -- Phase 2 (Inject): Cut MongoDB connection via Toxiproxy --
	t.Log("Phase 2 (Inject): Injecting complete MongoDB connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(mongoProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	t.Cleanup(func() {
		_ = chaosutil.RemoveAllToxics(mongoProxy)
	})

	t.Log("Phase 2 (Inject): MongoDB connection loss injected successfully")

	// -- Phase 3 (Verify Failure): Notifications endpoint should fail gracefully --
	t.Log("Phase 3 (Verify Failure): Waiting for connection loss to take effect...")
	time.Sleep(5 * time.Second)

	// Notifications endpoint depends on MongoDB (FindActiveNotifiable) and should return 500
	t.Log("Phase 3 (Verify Failure): Attempting notifications request during MongoDB connection loss...")
	reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3()

	code, body, err = cli.Request(reqCtx3, "GET", "/v1/deadlines/notifications", headers, nil)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines/notifications returned transport error: %v", err)
	} else {
		assert.NotEqual(t, 200, code,
			"Notifications endpoint should not return 200 during MongoDB connection loss (got %d)", code)
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines/notifications returned code=%d (expected non-200)", code)

		// If we got a response, verify it is a proper error response (not a panic stack trace)
		if code == 500 {
			var errResp map[string]string
			if json.Unmarshal(body, &errResp) == nil {
				assert.Contains(t, errResp, "error",
					"500 response should contain an 'error' field for graceful degradation")
				t.Logf("Phase 3 (Verify Failure): Error response: %s", string(body))
			}
		}
	}

	// Verify notifications with custom limit also fails gracefully
	t.Log("Phase 3 (Verify Failure): Attempting notifications with limit parameter...")
	reqCtx3b, cancel3b := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3b()

	code, _, err = cli.Request(reqCtx3b, "GET", "/v1/deadlines/notifications?limit=5", headers, nil)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines/notifications?limit=5 transport error: %v", err)
	} else {
		assert.NotEqual(t, 200, code,
			"Notifications with limit should not return 200 during MongoDB connection loss")
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines/notifications?limit=5 returned code=%d", code)
	}

	// CRITICAL: Service must still be alive - liveness probe must respond
	t.Log("Phase 3 (Verify Failure): Verifying service process is still alive...")
	reqCtxHealth, cancelHealth := context.WithTimeout(ctx, 5*time.Second)
	defer cancelHealth()

	code, _, err = cli.Request(reqCtxHealth, "GET", "/health", nil, nil)
	require.NoError(t, err, "Service liveness probe must respond during MongoDB connection loss (process may have crashed)")
	require.Equal(t, 200, code, "Service liveness probe must return 200 during MongoDB connection loss (graceful degradation required)")

	// -- Phase 4 (Restore): Remove toxics, restore MongoDB connectivity --
	t.Log("Phase 4 (Restore): Removing Toxiproxy toxics to restore MongoDB connectivity...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB connectivity restoration")
	t.Log("Phase 4 (Restore): System recovered, MongoDB connectivity restored")

	// -- Phase 5 (Recovery): Verify notifications endpoint returns valid data again --
	t.Log("Phase 5 (Recovery): Verifying notifications endpoint returns valid data after recovery...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, body, err = cli.Request(reqCtx5, "GET", "/v1/deadlines/notifications", headers, nil)
	require.NoError(t, err, "Notifications request should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications should return 200 after recovery")

	var recoveredResp notificationEndpointResponse
	require.NoError(t, json.Unmarshal(body, &recoveredResp), "Should unmarshal notifications response after recovery")
	assert.GreaterOrEqual(t, recoveredResp.Total, 0, "Total should be non-negative after recovery")
	t.Logf("Phase 5 (Recovery): Recovered notifications - total=%d", recoveredResp.Total)

	// Readiness probe should be fully healthy
	reqCtx5Ready, cancel5Ready := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Ready()

	code, _, err = cli.Request(reqCtx5Ready, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Readiness probe should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "Readiness probe should return 200 after MongoDB recovery")
	t.Log("Phase 5 (Recovery): Notifications endpoint fully operational after MongoDB connection loss recovery")
}

// --- CS-2: MongoDB High Latency during notification query ---

// TestIntegration_Chaos_Notification_MongoDB_HighLatency injects high latency into the MongoDB
// connection via Toxiproxy and validates that the notifications endpoint
// (GET /v1/deadlines/notifications) either times out gracefully or responds with degraded
// performance, without crashing.
//
// The notification handler calls FindActiveNotifiable which queries MongoDB. High latency
// should cause the query to slow down, potentially triggering context deadline exceeded errors.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestIntegration_Chaos_Notification_MongoDB_HighLatency(t *testing.T) {
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

	// -- Phase 1 (Normal): Verify notifications endpoint works with normal latency --
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB latency injection...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Measure baseline response time for notifications endpoint
	t.Log("Phase 1 (Normal): Measuring baseline notifications response time...")
	reqCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()

	startNormal := time.Now()

	code, body, err := cli.Request(reqCtx1, "GET", "/v1/deadlines/notifications", headers, nil)
	normalDuration := time.Since(startNormal)
	require.NoError(t, err, "Notifications request should succeed before chaos injection")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications should return 200 before chaos injection")

	var baselineResp notificationEndpointResponse
	require.NoError(t, json.Unmarshal(body, &baselineResp), "Should unmarshal baseline notifications")
	t.Logf("Phase 1 (Normal): Notifications returned in %v - total=%d", normalDuration, baselineResp.Total)

	// -- Phase 2 (Inject): Add 5000ms latency with 1000ms jitter to MongoDB --
	t.Log("Phase 2 (Inject): Injecting 5000ms latency + 1000ms jitter into MongoDB connection...")
	err = chaosutil.InjectLatency(mongoProxy, 5000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")

	t.Cleanup(func() {
		_ = chaosutil.RemoveAllToxics(mongoProxy)
	})

	t.Log("Phase 2 (Inject): MongoDB high latency injected successfully")

	// -- Phase 3 (Verify Failure): Notifications endpoint should timeout or be very slow --
	t.Log("Phase 3 (Verify Failure): Waiting for latency to take effect...")
	time.Sleep(3 * time.Second)

	// Attempt notifications request with a short timeout - should fail or be very slow
	t.Log("Phase 3 (Verify Failure): Attempting notifications request under high latency (3s timeout)...")
	reqCtx3Short, cancel3Short := context.WithTimeout(ctx, 3*time.Second)
	defer cancel3Short()

	startLatency := time.Now()

	code, _, err = cli.Request(reqCtx3Short, "GET", "/v1/deadlines/notifications", headers, nil)
	latencyDuration := time.Since(startLatency)

	// With 5000ms injected latency and 3s client timeout, request should either timeout or be noticeably slow
	latencyObserved := err != nil || latencyDuration > 2*time.Second
	assert.True(t, latencyObserved,
		"Latency injection should cause timeout or slow response, but completed in %v with no error", latencyDuration)

	if err != nil {
		t.Logf("Phase 3 (Verify Failure): Notifications request timed out as expected (duration=%v, err=%v)", latencyDuration, err)
	} else {
		t.Logf("Phase 3 (Verify Failure): Notifications request returned code=%d in %v (expected slow or timeout)", code, latencyDuration)
	}

	// Attempt with a longer timeout to see if the endpoint eventually responds
	t.Log("Phase 3 (Verify Failure): Attempting notifications request with longer timeout (15s)...")
	reqCtx3Long, cancel3Long := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3Long()

	code, _, err = cli.Request(reqCtx3Long, "GET", "/v1/deadlines/notifications", headers, nil)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): Notifications request failed even with 15s timeout: %v", err)
	} else {
		t.Logf("Phase 3 (Verify Failure): Notifications request returned code=%d with 15s timeout", code)
	}

	// Liveness probe should still respond (should not depend on MongoDB latency)
	reqCtx3Health, cancel3Health := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3Health()

	code, _, err = cli.Request(reqCtx3Health, "GET", "/health", nil, nil)
	require.NoError(t, err, "Service liveness probe must respond during MongoDB high latency (process may have crashed)")
	require.Equal(t, 200, code, "Service liveness probe must return 200 during MongoDB high latency (graceful degradation required)")

	// -- Phase 4 (Restore): Remove latency toxic, restore normal MongoDB operation --
	t.Log("Phase 4 (Restore): Removing latency toxic from MongoDB proxy...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB latency removal")
	t.Log("Phase 4 (Restore): System recovered, MongoDB latency removed")

	// -- Phase 5 (Recovery): Verify notifications endpoint returns to normal response times --
	t.Log("Phase 5 (Recovery): Verifying notifications endpoint restored to normal response times...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	startRecovery := time.Now()

	code, body, err = cli.Request(reqCtx5, "GET", "/v1/deadlines/notifications", headers, nil)
	recoveryDuration := time.Since(startRecovery)
	require.NoError(t, err, "Notifications request should succeed after latency removal")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications should return 200 after latency removal")

	var recoveredResp notificationEndpointResponse
	require.NoError(t, json.Unmarshal(body, &recoveredResp), "Should unmarshal recovered notifications")
	assert.GreaterOrEqual(t, recoveredResp.Total, 0, "Total should be non-negative after recovery")
	t.Logf("Phase 5 (Recovery): Notifications returned in %v (normal baseline was %v) - total=%d",
		recoveryDuration, normalDuration, recoveredResp.Total)

	// Readiness probe should be fully healthy
	reqCtx5Ready, cancel5Ready := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Ready()

	code, _, err = cli.Request(reqCtx5Ready, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Readiness probe should succeed after latency removal")
	require.Equal(t, 200, code, "Readiness probe should return 200 after latency removal")
	t.Log("Phase 5 (Recovery): Notifications endpoint fully restored to normal after high latency injection")
}

// --- CS-3: MongoDB Network Partition during notification query ---

// TestIntegration_Chaos_Notification_MongoDB_NetworkPartition simulates intermittent network
// partition on MongoDB via Toxiproxy packet loss and validates that the notifications endpoint
// (GET /v1/deadlines/notifications) either fails gracefully or returns degraded results,
// while the service stays alive and recovers after the partition is healed.
//
// Network partition with 90% packet loss tests the error handling path of FindActiveNotifiable
// when MongoDB connections are unreliable.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestIntegration_Chaos_Notification_MongoDB_NetworkPartition(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure.
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	mongoProxy, useToxiproxy := getMongoDBProxy()
	if !useToxiproxy {
		t.Skip("Skipping network partition test - Toxiproxy not available")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	// -- Phase 1 (Normal): Verify system is healthy and notifications endpoint works --
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB network partition...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify notifications endpoint works under normal conditions
	t.Log("Phase 1 (Normal): Verifying notifications endpoint under normal conditions...")
	reqCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()

	code, body, err := cli.Request(reqCtx1, "GET", "/v1/deadlines/notifications", headers, nil)
	require.NoError(t, err, "Notifications request should succeed before chaos injection")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications should return 200 before chaos injection")

	var baselineResp notificationEndpointResponse
	require.NoError(t, json.Unmarshal(body, &baselineResp), "Should unmarshal baseline notifications")
	t.Logf("Phase 1 (Normal): Baseline notifications - total=%d", baselineResp.Total)

	// -- Phase 2 (Inject): Simulate network partition with high packet loss --
	t.Log("Phase 2 (Inject): Injecting 90% packet loss to simulate MongoDB network partition...")
	err = chaosutil.InjectPacketLoss(mongoProxy, 90)
	require.NoError(t, err, "Failed to inject packet loss via Toxiproxy")

	t.Cleanup(func() {
		_ = chaosutil.RemoveAllToxics(mongoProxy)
	})

	t.Log("Phase 2 (Inject): MongoDB network partition injected (90% packet loss)")

	// -- Phase 3 (Verify Failure): Notifications endpoint should fail or degrade --
	t.Log("Phase 3 (Verify Failure): Waiting for network partition to take effect...")
	time.Sleep(5 * time.Second)

	// Attempt multiple notifications requests - with 90% packet loss, most should fail
	t.Log("Phase 3 (Verify Failure): Attempting notifications requests during network partition...")

	failureCount := 0
	totalAttempts := 3

	for i := 0; i < totalAttempts; i++ {
		reqCtx3, cancel3 := context.WithTimeout(ctx, 10*time.Second)

		code, _, err = cli.Request(reqCtx3, "GET", "/v1/deadlines/notifications", headers, nil)
		cancel3()

		if err != nil || code != 200 {
			failureCount++
			t.Logf("Phase 3 (Verify Failure): Attempt %d - notifications failed (code=%d, err=%v)", i+1, code, err)
		} else {
			t.Logf("Phase 3 (Verify Failure): Attempt %d - notifications succeeded despite partition (some packets got through)", i+1)
		}

		// Brief pause between attempts
		time.Sleep(1 * time.Second)
	}

	t.Logf("Phase 3 (Verify Failure): %d/%d notifications requests failed during network partition", failureCount, totalAttempts)
	assert.Greater(t, failureCount, 0,
		"At least one notifications request should fail during 90%% packet loss partition, but all %d succeeded", totalAttempts)

	// CRITICAL: Service must still be alive - liveness probe must respond
	t.Log("Phase 3 (Verify Failure): Verifying service process is still alive during partition...")
	reqCtx3Health, cancel3Health := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Health()

	code, _, err = cli.Request(reqCtx3Health, "GET", "/health", nil, nil)
	require.NoError(t, err, "Service liveness probe must respond during MongoDB network partition (process may have crashed)")
	require.Equal(t, 200, code, "Service liveness probe must return 200 during MongoDB network partition (graceful degradation required)")

	// -- Phase 4 (Restore): Remove packet loss, restore network --
	t.Log("Phase 4 (Restore): Removing packet loss toxic to restore MongoDB network...")
	err = chaosutil.RemoveAllToxics(mongoProxy)
	require.NoError(t, err, "Failed to remove toxics from MongoDB proxy")

	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "System did not recover after MongoDB network partition restoration")
	t.Log("Phase 4 (Restore): System recovered, MongoDB network restored")

	// -- Phase 5 (Recovery): Verify notifications endpoint returns valid data consistently --
	t.Log("Phase 5 (Recovery): Verifying notifications endpoint returns valid data after network partition...")
	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	code, body, err = cli.Request(reqCtx5, "GET", "/v1/deadlines/notifications", headers, nil)
	require.NoError(t, err, "Notifications request should succeed after network partition recovery")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications should return 200 after recovery")

	var recoveredResp notificationEndpointResponse
	require.NoError(t, json.Unmarshal(body, &recoveredResp), "Should unmarshal recovered notifications")
	assert.GreaterOrEqual(t, recoveredResp.Total, 0, "Total should be non-negative after recovery")
	t.Logf("Phase 5 (Recovery): Recovered notifications - total=%d", recoveredResp.Total)

	// Verify notifications with limit parameter also works after recovery
	reqCtx5b, cancel5b := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5b()

	code, _, err = cli.Request(reqCtx5b, "GET", "/v1/deadlines/notifications?limit=5", headers, nil)
	require.NoError(t, err, "Notifications with limit should succeed after recovery")
	require.Equal(t, 200, code, "GET /v1/deadlines/notifications?limit=5 should return 200 after recovery")

	// Readiness probe should be fully healthy
	reqCtx5Ready, cancel5Ready := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Ready()

	code, _, err = cli.Request(reqCtx5Ready, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Readiness probe should succeed after network partition recovery")
	require.Equal(t, 200, code, "Readiness probe should return 200 after network partition recovery")
	t.Log("Phase 5 (Recovery): Notifications endpoint fully operational after MongoDB network partition recovery")
}
