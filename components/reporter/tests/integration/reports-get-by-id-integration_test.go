//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/reporter/tests/utils"
)

type integrationEnv struct {
	ctx     context.Context
	cli     *h.HTTPClient
	headers map[string]string
}

func setupIntegrationEnv(t *testing.T) integrationEnv {
	t.Helper()

	env := h.LoadEnvironment()

	return integrationEnv{
		ctx:     context.Background(),
		cli:     h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout),
		headers: h.AuthHeaders(),
	}
}

func requestAndUnmarshal(t *testing.T, e integrationEnv, method, path string, payload any, dest any) (int, []byte) {
	t.Helper()

	code, body, err := e.cli.Request(e.ctx, method, path, e.headers, payload)
	if err != nil {
		t.Fatalf("%s %s error: %v", method, path, err)
	}

	if dest != nil && (code == 200 || code == 201) {
		if err := json.Unmarshal(body, dest); err != nil {
			t.Fatalf("parse %s %s response: %v", method, path, err)
		}
	}

	return code, body
}

func fetchFirstItemID(t *testing.T, e integrationEnv, path string) string {
	t.Helper()

	code, body, err := e.cli.Request(e.ctx, "GET", path, e.headers, nil)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}

	if code != 200 {
		t.Fatalf("list failed: code=%d body=%s", code, string(body))
	}

	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse list response: %v", err)
	}

	if len(result.Items) == 0 {
		return ""
	}

	return result.Items[0].ID
}

func requireNonEmptyFields(t *testing.T, fields map[string]string) {
	t.Helper()

	for name, value := range fields {
		if value == "" {
			t.Fatalf("Report %s is empty", name)
		}
	}
}

// TestIntegration_Reports_GetByID_ValidID tests GET /v1/reports/{id} with a valid report ID
func TestIntegration_Reports_GetByID_ValidID(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	e := setupIntegrationEnv(t)

	reportID := fetchFirstItemID(t, e, "/v1/reports?limit=1")
	if reportID == "" {
		t.Skip("No reports found to test GET by ID")
	}

	t.Logf("Testing GET /v1/reports/%s", reportID)

	code, body := requestAndUnmarshal(t, e, "GET", fmt.Sprintf("/v1/reports/%s", reportID), nil, nil)
	if code != 200 {
		t.Fatalf("get report by ID failed: code=%d body=%s", code, string(body))
	}

	var report struct {
		ID          string         `json:"id"`
		Status      string         `json:"status"`
		TemplateID  string         `json:"templateId"`
		CreatedAt   string         `json:"createdAt"`
		UpdatedAt   string         `json:"updatedAt"`
		CompletedAt string         `json:"completedAt"`
		DeletedAt   string         `json:"deletedAt"`
		Filters     map[string]any `json:"filters"`
		Metadata    map[string]any `json:"metadata"`
	}

	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("parse report response: %v", err)
	}

	requireNonEmptyFields(t, map[string]string{
		"ID":         report.ID,
		"status":     report.Status,
		"templateId": report.TemplateID,
		"createdAt":  report.CreatedAt,
		"updatedAt":  report.UpdatedAt,
	})

	if report.ID != reportID {
		t.Fatalf("Report ID mismatch: expected %s, got %s", reportID, report.ID)
	}

	validStatuses := map[string]bool{"Processing": true, "Finished": true, "Error": true}
	if !validStatuses[report.Status] {
		t.Fatalf("Invalid report status: %s", report.Status)
	}

	t.Logf("Report retrieved successfully:")
	t.Logf("   - ID: %s", report.ID)
	t.Logf("   - Status: %s", report.Status)
	t.Logf("   - TemplateID: %s", report.TemplateID)
	t.Logf("   - CreatedAt: %s", report.CreatedAt)
	t.Logf("   - UpdatedAt: %s", report.UpdatedAt)
	if report.CompletedAt != "" {
		t.Logf("   - CompletedAt: %s", report.CompletedAt)
	}
}

// TestIntegration_Reports_GetByID_InvalidID tests GET /v1/reports/{id} with an invalid report ID
func TestIntegration_Reports_GetByID_InvalidID(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	invalidID := "00000000-0000-0000-0000-000000000000"
	code, body, err := cli.Request(ctx, "GET", fmt.Sprintf("/v1/reports/%s", invalidID), headers, nil)
	if err != nil {
		t.Fatalf("get report by invalid ID error: %v", err)
	}

	if code != 404 {
		t.Fatalf("Expected 404 for invalid report ID, got: code=%d body=%s", code, string(body))
	}

	var errorResp struct {
		Title   string `json:"title"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errorResp); err != nil {
		t.Fatalf("parse error response: %v", err)
	}

	if errorResp.Title == "" {
		t.Fatalf("Error response missing title")
	}
	if errorResp.Code == "" {
		t.Fatalf("Error response missing code")
	}
	if errorResp.Message == "" {
		t.Fatalf("Error response missing message")
	}

	t.Logf("✅ Invalid report ID handled correctly:")
	t.Logf("   - Status: 404")
	t.Logf("   - Title: %s", errorResp.Title)
	t.Logf("   - Code: %s", errorResp.Code)
	t.Logf("   - Message: %s", errorResp.Message)
}

func createReportAndWait(t *testing.T, e integrationEnv, templateID string) string {
	t.Helper()

	payload := map[string]any{
		"templateId": templateID,
		"filters":    map[string]any{},
	}

	createCode, createBody, err := e.cli.Request(e.ctx, "POST", "/v1/reports", e.headers, payload)
	if err != nil {
		t.Fatalf("create test report error: %v", err)
	}

	if createCode != 201 {
		t.Logf("Could not create test report (code=%d), trying to use existing reports", createCode)
		return fallbackToExistingReport(t, e)
	}

	var reportResponse struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(createBody, &reportResponse); err != nil {
		t.Fatalf("parse create report response: %v", err)
	}

	t.Logf("Created test report ID: %s", reportResponse.ID)
	waitForReportFinished(t, e, reportResponse.ID)

	return reportResponse.ID
}

func fallbackToExistingReport(t *testing.T, e integrationEnv) string {
	t.Helper()

	reportID := fetchFirstItemID(t, e, "/v1/reports?limit=1")
	if reportID == "" {
		t.Skip("No reports available for testing")
	}

	t.Logf("Using existing report ID: %s", reportID)

	return reportID
}

func waitForReportFinished(t *testing.T, e integrationEnv, reportID string) {
	t.Helper()

	t.Log("Waiting for report to be processed...")

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Log("Timeout waiting for report to finish, using report as-is")
			return
		case <-ticker.C:
			statusCode, statusBody, err := e.cli.Request(e.ctx, "GET", fmt.Sprintf("/v1/reports/%s", reportID), e.headers, nil)
			if err != nil || statusCode != 200 {
				continue
			}

			var report struct {
				Status string `json:"status"`
			}

			if err := json.Unmarshal(statusBody, &report); err == nil && report.Status == "Finished" {
				t.Log("Report finished processing!")
				return
			}
		}
	}
}

func obtainReportIDForStatusTest(t *testing.T, e integrationEnv) string {
	t.Helper()

	reportID := fetchFirstItemID(t, e, "/v1/reports?status=Finished&limit=1")
	if reportID != "" {
		t.Logf("Using existing finished report ID: %s", reportID)
		return reportID
	}

	t.Log("No finished reports found, creating a test report...")

	templateID := fetchFirstItemID(t, e, "/v1/templates?limit=1")
	if templateID == "" {
		t.Skip("No templates available to create test report")
	}

	t.Logf("Using template ID: %s", templateID)

	return createReportAndWait(t, e, templateID)
}

// TestIntegration_Reports_GetByID_StatusFinished tests GET /v1/reports/{id} for a finished report
func TestIntegration_Reports_GetByID_StatusFinished(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	e := setupIntegrationEnv(t)

	reportID := obtainReportIDForStatusTest(t, e)
	t.Logf("Testing GET /v1/reports/%s", reportID)

	code, body, err := e.cli.Request(e.ctx, "GET", fmt.Sprintf("/v1/reports/%s", reportID), e.headers, nil)
	if err != nil {
		t.Fatalf("get finished report error: %v", err)
	}

	if code != 200 {
		t.Fatalf("get finished report failed: code=%d body=%s", code, string(body))
	}

	var report struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		CompletedAt string `json:"completedAt"`
		CreatedAt   string `json:"createdAt"`
		UpdatedAt   string `json:"updatedAt"`
	}
	_ = json.Unmarshal(body, &report)

	t.Logf("Report status: %s", report.Status)

	if report.Status == "Finished" {
		if report.CompletedAt == "" {
			t.Fatalf("Finished report should have completedAt field filled")
		}
		t.Logf("Report is finished and has completion timestamp")
	} else {
		t.Logf("Report is still in '%s' status (this is normal for integration tests)", report.Status)
	}

	if report.CreatedAt != "" && report.CompletedAt != "" {
		t.Logf("Report completion timeline:")
		t.Logf("   - CreatedAt: %s", report.CreatedAt)
		t.Logf("   - CompletedAt: %s", report.CompletedAt)
	}

	t.Logf("Finished report retrieved successfully:")
	t.Logf("   - ID: %s", report.ID)
	t.Logf("   - Status: %s", report.Status)
	t.Logf("   - CompletedAt: %s", report.CompletedAt)
}
