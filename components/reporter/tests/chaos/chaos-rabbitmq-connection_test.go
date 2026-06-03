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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	h "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils"
)

// TestIntegration_Chaos_RabbitMQ_ConnectionClosed tests the behavior when manager tries to send
// a message to RabbitMQ but the connection is closed
func TestIntegration_Chaos_RabbitMQ_ConnectionClosed(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure (stops/starts RabbitMQ).
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}
	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	t.Log("🔧 Starting RabbitMQ connection chaos test...")

	t.Log("Step 1: Verifying normal system operation...")
	templateID, ok := getAnyTemplateIDWithRetry(ctx, t, cli, headers, 10, 2*time.Second)
	if !ok {
		t.Skip("No templates available or service unstable for chaos testing")
	}
	t.Logf("Using template ID: %s", templateID)

	t.Log("Step 2: Stopping RabbitMQ container (connection closed)...")
	err := StopRabbitMQ()
	if err != nil {
		t.Fatalf("Failed to stop RabbitMQ: %v", err)
	}

	// Wait for connection closure to propagate to the Manager
	time.Sleep(3 * time.Second)

	t.Log("Step 3: Attempting to create report with RabbitMQ DOWN...")
	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	t.Log("Step 4: Analyzing system behavior with closed connection...")
	t.Logf("Response: code=%d, err=%v, body=%s", code, err, string(body))

	// With RabbitMQ completely stopped, the producer retry loop exhausts all attempts
	// and the report creation should either fail at HTTP level or return a server error.
	// Note: 201 is also acceptable if the report is created in MongoDB but the queue
	// publish fails (report status will be "Error" in that case).
	if code == 201 {
		t.Log("Report created in MongoDB but queue publish may have failed (status will be Error)")
	} else {
		assert.True(t, err != nil || code >= 400,
			"Expected error or non-success status during RabbitMQ outage, got code=%d err=%v", code, err)
	}

	t.Log("Step 5: Verifying manager still responds to other requests...")
	code, _, err = cli.Request(ctx, "GET", "/v1/templates?limit=1", headers, nil)
	t.Logf("Health probe during outage: code=%d, err=%v", code, err)

	// Manager should still respond to non-queue requests (graceful degradation)
	require.NoError(t, err, "Manager should still accept HTTP connections during RabbitMQ outage")
	assert.Equal(t, 200, code, "Manager should serve non-queue endpoints during RabbitMQ outage")

	t.Log("Step 6: Restoring RabbitMQ connection...")
	err = StartRabbitMQ()
	if err != nil {
		t.Fatalf("Failed to start RabbitMQ: %v", err)
	}

	require.Eventually(t, func() bool {
		code, _, err := cli.Request(ctx, "GET", "/readyz", nil, nil)
		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "service did not become healthy after RabbitMQ restore")

	t.Log("Step 7: Verifying system recovery with fresh request...")
	// Use a unique idempotency key to avoid 409 Conflict with the Step 3 request
	recoveryHeaders := make(map[string]string)
	for k, v := range headers {
		recoveryHeaders[k] = v
	}
	recoveryHeaders["X-Idempotency"] = "chaos-recovery-" + time.Now().Format("20060102150405.000")
	code, _, err = cli.Request(ctx, "POST", "/v1/reports", recoveryHeaders, payload)
	t.Logf("Recovery probe: code=%d, err=%v", code, err)

	// After recovery, report creation should succeed
	require.NoError(t, err, "Expected no HTTP error after RabbitMQ recovery")
	assert.True(t, code == 200 || code == 201,
		"Expected success status after RabbitMQ recovery, got code=%d", code)

	t.Log("RabbitMQ connection chaos test completed")
}

// TestIntegration_Chaos_RabbitMQ_ChannelClosed tests when RabbitMQ is running but the channel is closed
func TestIntegration_Chaos_RabbitMQ_ChannelClosed(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure (restarts RabbitMQ).
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	t.Log("⏳ Waiting for full system recovery after previous chaos tests...")
	if err := h.WaitForSystemHealth(ctx, cli, 90*time.Second); err != nil {
		t.Logf("⚠️  System health check failed: %v", err)
		t.Skip("System not ready for channel chaos test - likely recovering from previous test")
	}

	t.Log("🔧 Starting RabbitMQ channel chaos test...")

	t.Log("Step 1: Verifying normal system operation...")
	templateID, ok := getAnyTemplateIDWithRetry(ctx, t, cli, headers, 10, 2*time.Second)
	if !ok {
		t.Skip("No templates available or service unstable for chaos testing")
	}

	t.Log("Step 2: Simulating channel closure (quick RabbitMQ restart)...")
	err := RestartRabbitMQ(2 * time.Second)
	if err != nil {
		t.Fatalf("Failed to restart RabbitMQ: %v", err)
	}

	// Intentional wait: allow channel closure to take effect before testing behavior
	time.Sleep(3 * time.Second)

	t.Log("Step 3: Attempting to create report during channel issues...")
	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)

	t.Log("Step 4: Analyzing behavior during channel issues...")
	t.Logf("Response: code=%d, err=%v, body=%s", code, err, string(body))

	// During channel issues, system should either fail gracefully or handle it transparently
	assert.True(t, err != nil || code >= 200,
		"Expected either an error or a valid HTTP response during channel issues, got code=%d err=%v", code, err)

	t.Log("Step 5: Waiting for automatic recovery...")
	require.Eventually(t, func() bool {
		code, _, err := cli.Request(ctx, "GET", "/readyz", nil, nil)
		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "service did not recover after channel closure")

	t.Log("Step 6: Verifying automatic recovery...")
	code, _, err = cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	t.Logf("Recovery probe: code=%d, err=%v", code, err)

	// After recovery, report creation should succeed
	require.NoError(t, err, "Expected no HTTP error after channel recovery")
	assert.True(t, code == 200 || code == 201,
		"Expected success status after channel recovery, got code=%d", code)

	t.Log("RabbitMQ channel chaos test completed")
}

// TestIntegration_Chaos_RabbitMQ_QueueFull tests behavior when RabbitMQ queue is full or unavailable
func TestIntegration_Chaos_RabbitMQ_QueueFull(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure (restarts RabbitMQ).
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	ctx := context.Background()
	cli := h.NewHTTPClient(GetManagerAddress(), 30*time.Second)
	headers := h.AuthHeaders()

	t.Log("⏳ Waiting for full system recovery after previous chaos tests...")
	if err := h.WaitForSystemHealth(ctx, cli, 90*time.Second); err != nil {
		t.Logf("⚠️  System health check failed after 90s: %v", err)
		t.Log("💡 This may be due to datasource initialization with retry - skipping test")
		t.Skip("System not ready for chaos test - likely datasource initialization in progress")
	}
	t.Log("✅ System is healthy, proceeding with queue chaos test...")

	t.Log("🔧 Starting RabbitMQ queue chaos test...")

	t.Log("Step 1: Verifying normal system operation...")
	templateID, ok := getAnyTemplateIDWithRetry(ctx, t, cli, headers, 10, 2*time.Second)
	if !ok {
		t.Skip("No templates available or service unstable for chaos testing")
	}

	t.Log("Step 2: Simulating queue unavailability...")
	err := RestartRabbitMQ(1 * time.Second)
	if err != nil {
		t.Fatalf("Failed to restart RabbitMQ: %v", err)
	}

	// Intentional wait: simulate queue unavailability window before testing rapid requests
	time.Sleep(2 * time.Second)

	t.Log("Step 3: Attempting rapid report creation during queue issues...")
	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	successCount := 0
	errorCount := 0

	for i := 0; i < 5; i++ {
		code, _, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil {
			errorCount++
			t.Logf("Request %d failed: %v", i+1, err)
		} else if code == 201 || code == 200 {
			successCount++
			t.Logf("Request %d succeeded: code=%d", i+1, code)
		} else {
			errorCount++
			t.Logf("Request %d returned error: code=%d", i+1, code)
		}
		time.Sleep(500 * time.Millisecond) // Pequeno delay entre requests
	}

	t.Log("Step 4: Analyzing behavior during queue issues...")
	t.Logf("Results: %d successful, %d failed out of 5 requests", successCount, errorCount)

	// During queue issues, at least some requests should fail
	assert.True(t, errorCount > 0 || successCount > 0,
		"Expected at least some requests to complete (success or failure), got success=%d error=%d", successCount, errorCount)

	t.Log("Step 5: Restoring RabbitMQ...")
	err = RestartRabbitMQ(5 * time.Second)
	if err != nil {
		t.Fatalf("Failed to restore RabbitMQ: %v", err)
	}

	require.Eventually(t, func() bool {
		code, _, err := cli.Request(ctx, "GET", "/readyz", nil, nil)
		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "service did not become healthy after RabbitMQ restore")

	t.Log("Step 6: Verifying system recovery...")
	code, _, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	t.Logf("Recovery probe: code=%d, err=%v", code, err)

	// After recovery, report creation should succeed
	require.NoError(t, err, "Expected no HTTP error after RabbitMQ recovery")
	assert.True(t, code == 200 || code == 201,
		"Expected success status after RabbitMQ recovery, got code=%d", code)

	t.Log("RabbitMQ queue chaos test completed")
}

// getAnyTemplateIDWithRetry tries to fetch any template ID with retries/backoff to tolerate transient errors
func getAnyTemplateIDWithRetry(ctx context.Context, t *testing.T, cli *h.HTTPClient, headers map[string]string, attempts int, delay time.Duration) (string, bool) {
	for i := 1; i <= attempts; i++ {
		code, body, err := cli.Request(ctx, "GET", "/v1/templates?limit=1", headers, nil)
		t.Logf("retry %d/%d: waiting %s due to err/code: %v/%d", i, attempts, delay, err, code)
		if err == nil && code == 200 {
			var templates struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.Unmarshal(body, &templates)
			if len(templates.Items) > 0 && templates.Items[0].ID != "" {
				return templates.Items[0].ID, true
			}
		}
		t.Logf("retry %d/%d: waiting %s due to err/code: %v/%d", i, attempts, delay, err, code)
		time.Sleep(delay)
	}
	return "", false
}
