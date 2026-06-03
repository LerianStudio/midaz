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

	h "github.com/LerianStudio/reporter/tests/utils"
	chaosutil "github.com/LerianStudio/reporter/tests/utils/chaos"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadlineChaosResponse mirrors the deadline JSON returned by the API for chaos test assertions.
type deadlineChaosResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	DueDate     string `json:"dueDate"`
	Frequency   string `json:"frequency"`
	Active      bool   `json:"active"`
	Color       string `json:"color"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// deadlineChaosListResponse mirrors the paginated list response for chaos test assertions.
type deadlineChaosListResponse struct {
	Items []deadlineChaosResponse `json:"items"`
	Limit int                     `json:"limit"`
	Page  int                     `json:"page"`
}

// createDeadlineChaosPayload builds a valid deadline creation payload with a future dueDate.
func createDeadlineChaosPayload(name string) map[string]any {
	return map[string]any{
		"name":             name,
		"description":      "Chaos test deadline",
		"type":             "regulatory",
		"dueDate":          time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
		"frequency":        "monthly",
		"active":           true,
		"notifyDaysBefore": 5,
		"color":            "#FF5733",
	}
}

// TestChaos_Deadline_ConnectionLoss simulates a complete MongoDB connection loss via Toxiproxy
// and validates that deadline CRUD operations return errors gracefully (no panic/crash),
// then verifies full recovery after connectivity is restored.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_Deadline_ConnectionLoss(t *testing.T) {
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

	// ── Phase 1 (Normal): Verify system is healthy and deadline CRUD works ──
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB connection loss...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Create a deadline under normal conditions to prove CRUD works
	t.Log("Phase 1 (Normal): Creating a deadline under normal conditions...")
	payload := createDeadlineChaosPayload("chaos-connloss-baseline")

	reqCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()

	code, body, err := cli.Request(reqCtx1, "POST", "/v1/deadlines", headers, payload)
	require.NoError(t, err, "Deadline creation should succeed before chaos injection")
	require.Equal(t, 201, code, "POST /v1/deadlines should return 201 before chaos injection")

	var baselineDeadline deadlineChaosResponse
	require.NoError(t, json.Unmarshal(body, &baselineDeadline), "Should unmarshal deadline response")
	require.NotEmpty(t, baselineDeadline.ID, "Baseline deadline should have an ID")
	t.Logf("Phase 1 (Normal): Baseline deadline created with ID=%s", baselineDeadline.ID)

	// Verify listing deadlines works
	reqCtxList, cancelList := context.WithTimeout(ctx, 10*time.Second)
	defer cancelList()

	code, _, err = cli.Request(reqCtxList, "GET", "/v1/deadlines?limit=1", headers, nil)
	require.NoError(t, err, "Deadline listing should succeed before chaos injection")
	require.Equal(t, 200, code, "GET /v1/deadlines should return 200 before chaos injection")
	t.Log("Phase 1 (Normal): Deadline CRUD verified, system healthy")

	// ── Phase 2 (Inject): Cut MongoDB connection via Toxiproxy ──
	t.Log("Phase 2 (Inject): Injecting complete MongoDB connection loss via Toxiproxy...")
	err = chaosutil.InjectConnectionLoss(mongoProxy)
	require.NoError(t, err, "Failed to inject connection loss via Toxiproxy")

	t.Cleanup(func() {
		_ = chaosutil.RemoveAllToxics(mongoProxy)
	})

	t.Log("Phase 2 (Inject): MongoDB connection loss injected successfully")

	// ── Phase 3 (Verify Failure): Deadline CRUD should fail gracefully ──
	t.Log("Phase 3 (Verify Failure): Waiting for connection loss to take effect...")
	time.Sleep(5 * time.Second)

	// CREATE should fail gracefully (not panic)
	t.Log("Phase 3 (Verify Failure): Attempting deadline creation during connection loss...")
	createPayload := createDeadlineChaosPayload("chaos-connloss-during-outage")

	reqCtxCreate, cancelCreate := context.WithTimeout(ctx, 10*time.Second)
	defer cancelCreate()

	code, _, err = cli.Request(reqCtxCreate, "POST", "/v1/deadlines", headers, createPayload)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): POST /v1/deadlines returned transport error: %v", err)
	} else {
		assert.NotEqual(t, 201, code,
			"Deadline creation should not succeed during MongoDB connection loss (got %d)", code)
		t.Logf("Phase 3 (Verify Failure): POST /v1/deadlines returned code=%d (expected non-201)", code)
	}

	// READ (list) should fail gracefully
	t.Log("Phase 3 (Verify Failure): Attempting deadline listing during connection loss...")

	reqCtxRead, cancelRead := context.WithTimeout(ctx, 10*time.Second)
	defer cancelRead()

	code, _, err = cli.Request(reqCtxRead, "GET", "/v1/deadlines?limit=1", headers, nil)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines returned transport error: %v", err)
	} else {
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines returned code=%d", code)
	}

	// UPDATE should fail gracefully
	t.Log("Phase 3 (Verify Failure): Attempting deadline update during connection loss...")
	updatePayload := map[string]any{"name": "chaos-connloss-updated"}

	reqCtxUpdate, cancelUpdate := context.WithTimeout(ctx, 10*time.Second)
	defer cancelUpdate()

	code, _, err = cli.Request(reqCtxUpdate, "PATCH", "/v1/deadlines/"+baselineDeadline.ID, headers, updatePayload)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): PATCH /v1/deadlines/%s returned transport error: %v", baselineDeadline.ID, err)
	} else {
		assert.NotEqual(t, 200, code,
			"Deadline update should not succeed during MongoDB connection loss (got %d)", code)
		t.Logf("Phase 3 (Verify Failure): PATCH /v1/deadlines/%s returned code=%d (expected non-200)", baselineDeadline.ID, code)
	}

	// DELETE should fail gracefully
	t.Log("Phase 3 (Verify Failure): Attempting deadline deletion during connection loss...")

	reqCtxDelete, cancelDelete := context.WithTimeout(ctx, 10*time.Second)
	defer cancelDelete()

	code, _, err = cli.Request(reqCtxDelete, "DELETE", "/v1/deadlines/"+baselineDeadline.ID, headers, nil)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): DELETE /v1/deadlines/%s returned transport error: %v", baselineDeadline.ID, err)
	} else {
		assert.NotEqual(t, 204, code,
			"Deadline deletion should not succeed during MongoDB connection loss (got %d)", code)
		t.Logf("Phase 3 (Verify Failure): DELETE /v1/deadlines/%s returned code=%d (expected non-204)", baselineDeadline.ID, code)
	}

	// Service should still be alive (liveness probe should not crash)
	t.Log("Phase 3 (Verify Failure): Verifying service process is still alive...")

	reqCtxHealth, cancelHealth := context.WithTimeout(ctx, 5*time.Second)
	defer cancelHealth()

	code, _, err = cli.Request(reqCtxHealth, "GET", "/health", nil, nil)
	require.NoError(t, err, "Service liveness probe must respond during MongoDB connection loss (process may have crashed)")
	require.Equal(t, 200, code, "Service liveness probe must return 200 during MongoDB connection loss (graceful degradation required)")

	// ── Phase 4 (Restore): Remove toxics, restore MongoDB connectivity ──
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

	// ── Phase 5 (Recovery): Verify deadline CRUD operations resume ──
	t.Log("Phase 5 (Recovery): Verifying deadline CRUD operations resume after recovery...")

	// CREATE after recovery
	recoveryPayload := createDeadlineChaosPayload("chaos-connloss-recovery")

	reqCtx5Create, cancel5Create := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Create()

	code, body, err = cli.Request(reqCtx5Create, "POST", "/v1/deadlines", headers, recoveryPayload)
	require.NoError(t, err, "Deadline creation should succeed after MongoDB recovery")
	require.Equal(t, 201, code, "POST /v1/deadlines should return 201 after recovery")

	var recoveredDeadline deadlineChaosResponse
	require.NoError(t, json.Unmarshal(body, &recoveredDeadline), "Should unmarshal recovered deadline")
	require.NotEmpty(t, recoveredDeadline.ID, "Recovered deadline should have an ID")
	t.Logf("Phase 5 (Recovery): Deadline created after recovery with ID=%s", recoveredDeadline.ID)

	// READ after recovery
	reqCtx5Read, cancel5Read := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Read()

	code, _, err = cli.Request(reqCtx5Read, "GET", "/v1/deadlines?limit=10", headers, nil)
	require.NoError(t, err, "Deadline listing should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "GET /v1/deadlines should return 200 after recovery")

	// Readiness probe should be fully healthy
	reqCtx5Ready, cancel5Ready := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Ready()

	code, _, err = cli.Request(reqCtx5Ready, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Readiness probe should succeed after MongoDB recovery")
	require.Equal(t, 200, code, "Readiness probe should return 200 after MongoDB recovery")
	t.Log("Phase 5 (Recovery): Deadline CRUD fully operational after MongoDB connection loss recovery")
}

// TestChaos_Deadline_HighLatency injects high latency into the MongoDB connection via Toxiproxy
// and validates that deadline operations timeout gracefully with proper errors, then verifies
// normal response times after latency removal.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_Deadline_HighLatency(t *testing.T) {
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

	// ── Phase 1 (Normal): Verify system is healthy and deadline CRUD works with normal latency ──
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB latency injection...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Verify deadline listing under normal conditions and measure baseline response time
	t.Log("Phase 1 (Normal): Verifying deadline listing under normal conditions...")

	reqCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()

	startNormal := time.Now()

	code, _, err := cli.Request(reqCtx1, "GET", "/v1/deadlines?limit=1", headers, nil)
	normalDuration := time.Since(startNormal)
	require.NoError(t, err, "Deadline listing should succeed before chaos injection")
	require.Equal(t, 200, code, "GET /v1/deadlines should return 200 before chaos injection")
	t.Logf("Phase 1 (Normal): Deadline listing completed in %v", normalDuration)

	// Create a baseline deadline
	t.Log("Phase 1 (Normal): Creating baseline deadline under normal conditions...")
	payload := createDeadlineChaosPayload("chaos-latency-baseline")

	reqCtx1Create, cancel1Create := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1Create()

	code, body, err := cli.Request(reqCtx1Create, "POST", "/v1/deadlines", headers, payload)
	require.NoError(t, err, "Deadline creation should succeed before chaos injection")
	require.Equal(t, 201, code, "POST /v1/deadlines should return 201 before chaos injection")

	var baselineDeadline deadlineChaosResponse
	require.NoError(t, json.Unmarshal(body, &baselineDeadline), "Should unmarshal baseline deadline")
	t.Logf("Phase 1 (Normal): Baseline deadline created with ID=%s", baselineDeadline.ID)

	// ── Phase 2 (Inject): Add 5000ms latency with 1000ms jitter to MongoDB ──
	t.Log("Phase 2 (Inject): Injecting 5000ms latency + 1000ms jitter into MongoDB connection...")
	err = chaosutil.InjectLatency(mongoProxy, 5000, 1000)
	require.NoError(t, err, "Failed to inject latency via Toxiproxy")

	t.Cleanup(func() {
		_ = chaosutil.RemoveAllToxics(mongoProxy)
	})

	t.Log("Phase 2 (Inject): MongoDB high latency injected successfully")

	// ── Phase 3 (Verify Failure): Deadline operations should timeout or be very slow ──
	t.Log("Phase 3 (Verify Failure): Waiting for latency to take effect...")
	time.Sleep(3 * time.Second)

	// Attempt deadline listing with a short timeout - should fail or be very slow
	t.Log("Phase 3 (Verify Failure): Attempting deadline listing under high latency...")

	reqCtx3Short, cancel3Short := context.WithTimeout(ctx, 3*time.Second)
	defer cancel3Short()

	startLatency := time.Now()

	code, _, err = cli.Request(reqCtx3Short, "GET", "/v1/deadlines?limit=1", headers, nil)
	latencyDuration := time.Since(startLatency)

	// With 5000ms injected latency and 3s timeout, the request should either timeout or take noticeably longer
	latencyObserved := err != nil || latencyDuration > 2*time.Second
	assert.True(t, latencyObserved,
		"Latency injection should cause timeout or slow response, but completed in %v with no error", latencyDuration)

	if err != nil {
		t.Logf("Phase 3 (Verify Failure): Deadline listing timed out as expected (duration=%v, err=%v)", latencyDuration, err)
	} else {
		t.Logf("Phase 3 (Verify Failure): Deadline listing returned code=%d in %v (expected slow or timeout)", code, latencyDuration)
	}

	// Attempt deadline creation under latency with a longer timeout
	t.Log("Phase 3 (Verify Failure): Attempting deadline creation under high latency...")
	latencyPayload := createDeadlineChaosPayload("chaos-latency-during-injection")

	reqCtx3Create, cancel3Create := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3Create()

	code, _, err = cli.Request(reqCtx3Create, "POST", "/v1/deadlines", headers, latencyPayload)
	if err != nil {
		t.Logf("Phase 3 (Verify Failure): POST /v1/deadlines timed out or failed: %v", err)
	} else {
		t.Logf("Phase 3 (Verify Failure): POST /v1/deadlines returned code=%d under high latency", code)
	}

	// Liveness probe should still respond (should not depend on MongoDB latency)
	reqCtx3Health, cancel3Health := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3Health()

	code, _, err = cli.Request(reqCtx3Health, "GET", "/health", nil, nil)
	require.NoError(t, err, "Service liveness probe must respond during MongoDB high latency (process may have crashed)")
	require.Equal(t, 200, code, "Service liveness probe must return 200 during MongoDB high latency (graceful degradation required)")

	// ── Phase 4 (Restore): Remove latency toxic, restore normal MongoDB operation ──
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

	// ── Phase 5 (Recovery): Verify deadline operations return to normal response times ──
	t.Log("Phase 5 (Recovery): Verifying deadline operations restored to normal response times...")

	reqCtx5, cancel5 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5()

	startRecovery := time.Now()

	code, _, err = cli.Request(reqCtx5, "GET", "/v1/deadlines?limit=1", headers, nil)
	recoveryDuration := time.Since(startRecovery)
	require.NoError(t, err, "Deadline listing should succeed after latency removal")
	require.Equal(t, 200, code, "GET /v1/deadlines should return 200 after latency removal")
	t.Logf("Phase 5 (Recovery): Deadline listing completed in %v (normal baseline was %v)", recoveryDuration, normalDuration)

	// Verify deadline creation works at normal speed
	recoveryPayload := createDeadlineChaosPayload("chaos-latency-recovery")

	reqCtx5Create, cancel5Create := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Create()

	code, body, err = cli.Request(reqCtx5Create, "POST", "/v1/deadlines", headers, recoveryPayload)
	require.NoError(t, err, "Deadline creation should succeed after latency removal")
	require.Equal(t, 201, code, "POST /v1/deadlines should return 201 after latency removal")

	var recoveredDeadline deadlineChaosResponse
	require.NoError(t, json.Unmarshal(body, &recoveredDeadline), "Should unmarshal recovered deadline")
	require.NotEmpty(t, recoveredDeadline.ID, "Recovered deadline should have an ID")

	// Readiness probe should be fully healthy
	reqCtx5Ready, cancel5Ready := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Ready()

	code, _, err = cli.Request(reqCtx5Ready, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Readiness probe should succeed after latency removal")
	require.Equal(t, 200, code, "Readiness probe should return 200 after latency removal")
	t.Log("Phase 5 (Recovery): Deadline operations fully restored to normal after high latency injection")
}

// TestChaos_Deadline_NetworkPartition simulates intermittent network partition on MongoDB
// via Toxiproxy packet loss and validates that the service stays alive, returns errors to
// clients, and recovers after the partition is healed.
//
// 5-phase structure:
// Phase 1 (Normal) -> Phase 2 (Inject) -> Phase 3 (Verify Failure) -> Phase 4 (Restore) -> Phase 5 (Recovery)
func TestChaos_Deadline_NetworkPartition(t *testing.T) {
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

	// ── Phase 1 (Normal): Verify system is healthy and deadline operations work ──
	t.Log("Phase 1 (Normal): Verifying system health before MongoDB network partition...")
	require.Eventually(t, func() bool {
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		code, _, err := cli.Request(reqCtx, "GET", "/readyz", nil, nil)

		return err == nil && code == 200
	}, 60*time.Second, 2*time.Second, "System not healthy before chaos injection")

	// Create a baseline deadline before partition
	t.Log("Phase 1 (Normal): Creating baseline deadline before network partition...")
	payload := createDeadlineChaosPayload("chaos-partition-baseline")

	reqCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()

	code, body, err := cli.Request(reqCtx1, "POST", "/v1/deadlines", headers, payload)
	require.NoError(t, err, "Deadline creation should succeed before chaos injection")
	require.Equal(t, 201, code, "POST /v1/deadlines should return 201 before chaos injection")

	var baselineDeadline deadlineChaosResponse
	require.NoError(t, json.Unmarshal(body, &baselineDeadline), "Should unmarshal baseline deadline")
	t.Logf("Phase 1 (Normal): Baseline deadline created with ID=%s", baselineDeadline.ID)

	// ── Phase 2 (Inject): Simulate network partition with high packet loss ──
	t.Log("Phase 2 (Inject): Injecting 90% packet loss to simulate MongoDB network partition...")
	err = chaosutil.InjectPacketLoss(mongoProxy, 90)
	require.NoError(t, err, "Failed to inject packet loss via Toxiproxy")

	t.Cleanup(func() {
		_ = chaosutil.RemoveAllToxics(mongoProxy)
	})

	t.Log("Phase 2 (Inject): MongoDB network partition injected (90% packet loss)")

	// ── Phase 3 (Verify Failure): Service should stay alive, deadline ops should fail/degrade ──
	t.Log("Phase 3 (Verify Failure): Waiting for network partition to take effect...")
	time.Sleep(5 * time.Second)

	// Attempt multiple deadline operations - they should fail or degrade
	t.Log("Phase 3 (Verify Failure): Attempting deadline CRUD during network partition...")

	failureCount := 0
	totalOps := 0

	// CREATE during partition
	createPayload := createDeadlineChaosPayload("chaos-partition-during")

	reqCtx3Create, cancel3Create := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Create()

	totalOps++

	code, _, err = cli.Request(reqCtx3Create, "POST", "/v1/deadlines", headers, createPayload)
	if err != nil || code != 201 {
		failureCount++
		t.Logf("Phase 3 (Verify Failure): POST /v1/deadlines failed as expected (code=%d, err=%v)", code, err)
	} else {
		t.Log("Phase 3 (Verify Failure): POST /v1/deadlines succeeded despite partition (some packets got through)")
	}

	// READ during partition
	reqCtx3Read, cancel3Read := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Read()

	totalOps++

	code, _, err = cli.Request(reqCtx3Read, "GET", "/v1/deadlines?limit=1", headers, nil)
	if err != nil || code != 200 {
		failureCount++
		t.Logf("Phase 3 (Verify Failure): GET /v1/deadlines failed as expected (code=%d, err=%v)", code, err)
	} else {
		t.Log("Phase 3 (Verify Failure): GET /v1/deadlines succeeded despite partition (some packets got through)")
	}

	// UPDATE during partition
	updatePayload := map[string]any{"name": "chaos-partition-updated"}

	reqCtx3Update, cancel3Update := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Update()

	totalOps++

	code, _, err = cli.Request(reqCtx3Update, "PATCH", "/v1/deadlines/"+baselineDeadline.ID, headers, updatePayload)
	if err != nil || code != 200 {
		failureCount++
		t.Logf("Phase 3 (Verify Failure): PATCH /v1/deadlines/%s failed as expected (code=%d, err=%v)", baselineDeadline.ID, code, err)
	} else {
		t.Logf("Phase 3 (Verify Failure): PATCH /v1/deadlines/%s succeeded despite partition", baselineDeadline.ID)
	}

	t.Logf("Phase 3 (Verify Failure): %d/%d operations failed during network partition", failureCount, totalOps)

	assert.Greater(t, failureCount, 0,
		"At least one operation should fail during 90%% packet loss partition, but all %d succeeded", totalOps)

	// CRITICAL: Service must still be alive - liveness probe must respond
	t.Log("Phase 3 (Verify Failure): Verifying service process is still alive during partition...")

	reqCtx3Health, cancel3Health := context.WithTimeout(ctx, 10*time.Second)
	defer cancel3Health()

	code, _, err = cli.Request(reqCtx3Health, "GET", "/health", nil, nil)
	require.NoError(t, err, "Service liveness probe must respond during MongoDB network partition (process may have crashed)")
	require.Equal(t, 200, code, "Service liveness probe must return 200 during MongoDB network partition (graceful degradation required)")

	// ── Phase 4 (Restore): Remove packet loss, restore network ──
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

	// ── Phase 5 (Recovery): Verify deadline CRUD operations resume fully ──
	t.Log("Phase 5 (Recovery): Verifying deadline CRUD operations resume after network partition...")

	// CREATE after recovery
	recoveryPayload := createDeadlineChaosPayload("chaos-partition-recovery")

	reqCtx5Create, cancel5Create := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Create()

	code, body, err = cli.Request(reqCtx5Create, "POST", "/v1/deadlines", headers, recoveryPayload)
	require.NoError(t, err, "Deadline creation should succeed after network partition recovery")
	require.Equal(t, 201, code, "POST /v1/deadlines should return 201 after recovery")

	var recoveredDeadline deadlineChaosResponse
	require.NoError(t, json.Unmarshal(body, &recoveredDeadline), "Should unmarshal recovered deadline")
	require.NotEmpty(t, recoveredDeadline.ID, "Recovered deadline should have an ID")
	t.Logf("Phase 5 (Recovery): Deadline created after recovery with ID=%s", recoveredDeadline.ID)

	// READ after recovery - should return full list
	reqCtx5Read, cancel5Read := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Read()

	code, body, err = cli.Request(reqCtx5Read, "GET", "/v1/deadlines?limit=10", headers, nil)
	require.NoError(t, err, "Deadline listing should succeed after network partition recovery")
	require.Equal(t, 200, code, "GET /v1/deadlines should return 200 after recovery")

	var listResp deadlineChaosListResponse
	require.NoError(t, json.Unmarshal(body, &listResp), "Should unmarshal deadline list")
	assert.Greater(t, len(listResp.Items), 0, "Should have deadlines in list after recovery")
	t.Logf("Phase 5 (Recovery): Deadline listing returned %d items", len(listResp.Items))

	// Readiness probe should be fully healthy
	reqCtx5Ready, cancel5Ready := context.WithTimeout(ctx, 10*time.Second)
	defer cancel5Ready()

	code, _, err = cli.Request(reqCtx5Ready, "GET", "/readyz", nil, nil)
	require.NoError(t, err, "Readiness probe should succeed after network partition recovery")
	require.Equal(t, 200, code, "Readiness probe should return 200 after network partition recovery")
	t.Log("Phase 5 (Recovery): Deadline CRUD fully operational after MongoDB network partition recovery")
}
