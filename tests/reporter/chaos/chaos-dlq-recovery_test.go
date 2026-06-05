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

	"github.com/stretchr/testify/require"

	h "github.com/LerianStudio/midaz/v4/tests/reporter/utils"
)

func fetchFirstTemplateID(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string) string {
	t.Helper()

	listCode, listBody, err := cli.Request(ctx, "GET", "/v1/templates?limit=1", headers, nil)
	if err != nil {
		t.Fatalf("Failed to list templates: %v", err)
	}

	if listCode != 200 {
		t.Fatalf("Failed to list templates (HTTP %d): %s", listCode, string(listBody))
	}

	var templateList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}

	if err := json.Unmarshal(listBody, &templateList); err != nil {
		t.Fatalf("Error decoding template list: %v", err)
	}

	if len(templateList.Items) == 0 {
		t.Skip("No templates found in system - skipping test. Create a template first.")
	}

	return templateList.Items[0].ID
}

func createReport(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string, payload map[string]any) string {
	t.Helper()

	code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	if code != 200 && code != 201 {
		t.Fatalf("Expected 200/201, got %d: %s", code, string(body))
	}

	var reportResponse struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(body, &reportResponse); err != nil {
		t.Fatalf("Error decoding response: %v", err)
	}

	return reportResponse.ID
}

func logDLQFinalStatus(t *testing.T, ctx context.Context, cli *h.HTTPClient, reportID string, headers map[string]string) {
	t.Helper()

	finalReport, err := cli.WaitForReportStatus(ctx, reportID, headers, "Finished", 60*time.Second)
	if err != nil {
		currentReport, err2 := cli.GetReportStatus(ctx, reportID, headers)
		if err2 != nil {
			t.Fatalf("Error fetching final status: %v", err2)
		}

		t.Logf("Final report status: %s", currentReport.Status)

		switch currentReport.Status {
		case "Processing":
			t.Error("FAILURE: Report stuck in 'Processing' status")
			t.Error("This indicates the message was lost or not reprocessed")
			t.Error("DLQ/DLX implementation may not be working correctly")
		case "Error":
			t.Log("Report status updated to 'Error' (message was reprocessed)")
		default:
			t.Logf("Unexpected final status: %s", currentReport.Status)
		}
	} else {
		t.Log("SUCCESS: Report processed successfully after RabbitMQ recovery!")
		t.Logf("Final status: %s", finalReport.Status)
		t.Log("Message persisted through RabbitMQ crash and was reprocessed")
	}
}

// TestIntegration_Chaos_DLQ_RecoveryAfterRabbitMQFailure tests that messages are not lost when RabbitMQ crashes
func TestIntegration_Chaos_DLQ_RecoveryAfterRabbitMQFailure(t *testing.T) {
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

	t.Log("Step 1: Verifying system health...")
	if err := h.WaitForSystemHealth(ctx, cli, 70*time.Second); err != nil {
		t.Fatalf("System not healthy: %v", err)
	}
	t.Log("System is healthy")

	t.Log("Step 2: Fetching existing template...")
	templateID := fetchFirstTemplateID(t, ctx, cli, headers)
	t.Logf("Using existing template: %s", templateID)

	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	t.Log("Step 3: Creating report via Manager...")
	reportID := createReport(t, ctx, cli, headers, payload)
	t.Logf("Report created successfully! ID: %s", reportID)

	t.Log("Verifying initial report status...")
	initialReport, err := cli.GetReportStatus(ctx, reportID, headers)
	if err != nil {
		t.Fatalf("Failed to get report status: %v", err)
	}
	t.Logf("Initial status: %s", initialReport.Status)

	// Intentional wait: allow time for RabbitMQ publish to complete before stopping the broker
	t.Log("Waiting for message to be published to RabbitMQ (2s)...")
	time.Sleep(2 * time.Second)

	// CHAOS: Crash RabbitMQ
	t.Log("Step 4: CHAOS - Stopping RabbitMQ (simulating crash)...")
	if err := StopRabbitMQ(); err != nil {
		t.Fatalf("Failed to stop RabbitMQ: %v", err)
	}
	t.Log("RabbitMQ stopped (simulating crash)")

	// Intentional wait: simulate actual downtime period to test behavior during outage
	t.Log("Simulating downtime (5 seconds)...")
	time.Sleep(5 * time.Second)

	t.Log("Step 5: Checking report status during RabbitMQ downtime...")
	downtimeReport, err := cli.GetReportStatus(ctx, reportID, headers)
	if err != nil {
		t.Logf("Could not fetch report during downtime: %v", err)
	} else {
		t.Logf("Status during downtime: %s", downtimeReport.Status)
	}

	// RECOVERY: Restart RabbitMQ
	t.Log("Step 6: RECOVERY - Starting RabbitMQ...")
	if err := StartRabbitMQ(); err != nil {
		t.Fatalf("Failed to start RabbitMQ: %v", err)
	}

	t.Log("Waiting for RabbitMQ to fully initialize and worker to reconnect...")
	require.Eventually(t, func() bool {
		code, _, err := cli.Request(ctx, "GET", "/readyz", nil, nil)
		return err == nil && code == 200
	}, 90*time.Second, 2*time.Second, "system did not recover after RabbitMQ restart")
	t.Log("RabbitMQ started and system recovered")

	t.Log("Step 8: Checking final report status...")
	t.Log("Waiting up to 60 seconds for message to be reprocessed...")
	logDLQFinalStatus(t, ctx, cli, reportID, headers)

	// Final verification
	finalReport, _ := cli.GetReportStatus(ctx, reportID, headers)
	if finalReport.Status == "Processing" {
		t.Fatalf("TEST FAILED: Message was lost - report stuck in Processing")
	}

	t.Log("TEST PASSED: Message was not lost during RabbitMQ failure")
}
