//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	h "github.com/LerianStudio/reporter/tests/utils"
)

func assertNoServerError(t *testing.T, label string, code int, body []byte) {
	t.Helper()

	if code >= 500 {
		t.Fatalf("SERVER ERROR on %s: code=%d body=%s", label, code, string(body))
	}
}

func unmarshalID(body []byte) string {
	var resp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}

	return resp.ID
}

func tryGenerateReport(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string, templateID, templateName, testOrgID string) {
	t.Helper()

	payload := map[string]any{
		"templateId": templateID,
		"filters": map[string]any{
			"midaz_onboarding": map[string]any{
				"organization": map[string]any{
					"id": map[string]any{
						"eq": []string{testOrgID},
					},
				},
			},
		},
	}

	reportCode, reportBody, reportErr := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
	if reportErr != nil {
		t.Logf("Report generation request failed: %v", reportErr)
		return
	}

	assertNoServerError(t, "report generation for "+templateName, reportCode, reportBody)

	if reportCode != 200 && reportCode != 201 {
		t.Logf("Report creation rejected for %s: code=%d", templateName, reportCode)
		return
	}

	reportID := unmarshalID(reportBody)
	if reportID == "" {
		return
	}

	t.Logf("Report created: %s, waiting for processing...", reportID)
	checkReportStatus(t, ctx, cli, headers, reportID, templateName)
}

func checkReportStatus(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string, reportID, templateName string) {
	t.Helper()

	time.Sleep(5 * time.Second)

	statusCode, statusBody, _ := cli.Request(ctx, "GET", "/v1/reports/"+reportID, headers, nil)
	assertNoServerError(t, "status check for "+templateName, statusCode, statusBody)

	if statusCode != 200 {
		return
	}

	var statusResp struct {
		Status string `json:"status"`
	}

	if err := json.Unmarshal(statusBody, &statusResp); err != nil {
		return
	}

	t.Logf("Report status for %s: %s", templateName, statusResp.Status)

	if statusResp.Status == "Finished" {
		t.Errorf("Template %s unexpectedly succeeded but was expected to fail", templateName)
	}
}

// TestPredefined_InvalidTemplates tests pre-defined templates that should fail gracefully.
// NOTE: This is a deterministic robustness test with predefined template files, not a native Go fuzz test.
// Native fuzz tests use *testing.F with f.Add() seed corpus and f.Fuzz() for random input generation.
func TestPredefined_InvalidTemplates(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"

	templatesDir := "./templates"
	templateFiles, err := filepath.Glob(filepath.Join(templatesDir, "*.tpl"))
	if err != nil {
		t.Fatalf("Failed to find template files: %v", err)
	}

	if len(templateFiles) == 0 {
		t.Skip("No template files found in ./templates directory")
	}

	for _, templateFile := range templateFiles {
		templateName := filepath.Base(templateFile)

		t.Run(templateName, func(t *testing.T) {
			content, err := os.ReadFile(templateFile)
			if err != nil {
				t.Fatalf("Failed to read template %s: %v", templateName, err)
			}

			t.Logf("Testing invalid template: %s", templateName)

			files := map[string][]byte{
				"template": content,
			}

			formData := map[string]string{
				"outputFormat": "TXT",
				"description":  "Invalid template test: " + templateName,
			}

			code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
			if err != nil {
				t.Logf("Request error (may be expected): %v", err)
				return
			}

			assertNoServerError(t, "template "+templateName, code, body)

			if code != 200 && code != 201 {
				t.Logf("Template %s rejected at creation: code=%d", templateName, code)
				return
			}

			templateID := unmarshalID(body)
			if templateID == "" {
				return
			}

			t.Logf("Template %s accepted with ID: %s", templateName, templateID)
			tryGenerateReport(t, ctx, cli, headers, templateID, templateName, testOrgID)
		})
	}
}
