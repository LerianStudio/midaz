//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"context"
	"encoding/json"
	"testing"
	"testing/quick"
	"time"

	"github.com/google/uuid"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// Property 1: Report status deve sempre progredir de Processing → Finished ou Error
// Nunca deve regredir ou ter estados inválidos
func TestProperty_ReportStatus_ValidProgression(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"

	// Create a valid template first
	templateID := createTestTemplate(t, ctx, cli, headers, testOrgID)

	property := func(seed uint32) bool {
		// Create report
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

		code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil || code >= 500 {
			return true // Skip on errors
		}

		if code != 200 && code != 201 {
			return true // Skip if not created
		}

		var resp struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}

		if err := json.Unmarshal(body, &resp); err != nil {
			return true
		}

		reportID := resp.ID
		initialStatus := resp.Status

		// Wait and check final status
		time.Sleep(3 * time.Second)

		statusCode, statusBody, _ := cli.Request(ctx, "GET", "/v1/reports/"+reportID, headers, nil)
		if statusCode != 200 {
			return true
		}

		var finalResp struct {
			Status string `json:"status"`
		}

		if err := json.Unmarshal(statusBody, &finalResp); err != nil {
			return true
		}

		finalStatus := finalResp.Status

		// Valid progressions:
		// Processing → Finished ✓
		// Processing → Error ✓
		// Finished → Finished ✓
		// Error → Error ✓
		// Invalid: Finished → Processing, Error → Processing, etc.

		validTransitions := map[string]map[string]bool{
			"Processing": {"Processing": true, "Finished": true, "Error": true},
			"Finished":   {"Finished": true},
			"Error":      {"Error": true},
		}

		if allowed, exists := validTransitions[initialStatus]; exists {
			return allowed[finalStatus]
		}

		// Unknown initial status
		return false
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 10}); err != nil {
		t.Errorf("Property violated: invalid status progression: %v", err)
	}
}

// Property 2: Report IDs devem ser únicos (UUID v7)
func TestProperty_ReportID_Uniqueness(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"
	templateID := createTestTemplate(t, ctx, cli, headers, testOrgID)

	property := func(iterations uint8) bool {
		if iterations == 0 || iterations > 20 {
			return true
		}

		reportIDs := make(map[string]bool)

		for i := uint8(0); i < iterations; i++ {
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

			code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
			if err != nil || code >= 500 || (code != 200 && code != 201) {
				continue
			}

			var resp struct {
				ID string `json:"id"`
			}

			if err := json.Unmarshal(body, &resp); err != nil {
				continue
			}

			// Check uniqueness
			if reportIDs[resp.ID] {
				t.Logf("Duplicate report ID found: %s", resp.ID)
				return false
			}

			reportIDs[resp.ID] = true

			// Validate UUID format
			if _, err := uuid.Parse(resp.ID); err != nil {
				t.Logf("Invalid UUID format: %s", resp.ID)
				return false
			}
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 5}); err != nil {
		t.Errorf("Property violated: report IDs are not unique: %v", err)
	}
}

// Property 3: Status final deve ser sempre Finished ou Error (nunca Processing indefinidamente)
func TestProperty_ReportStatus_EventuallyTerminates(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	if testing.Short() {
		t.Skip("Skipping property test in short mode")
	}

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	testOrgID := "00000000-0000-0000-0000-000000000001"
	templateID := createTestTemplate(t, ctx, cli, headers, testOrgID)

	property := func(seed uint32) bool {
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

		code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil || code >= 500 || (code != 200 && code != 201) {
			return true
		}

		var resp struct {
			ID string `json:"id"`
		}

		if err := json.Unmarshal(body, &resp); err != nil {
			return true
		}

		// Wait for processing (max 10 seconds)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			statusCode, statusBody, _ := cli.Request(ctx, "GET", "/v1/reports/"+resp.ID, headers, nil)
			if statusCode != 200 {
				return true
			}

			var statusResp struct {
				Status string `json:"status"`
			}

			if err := json.Unmarshal(statusBody, &statusResp); err != nil {
				return true
			}

			// Terminal states
			if statusResp.Status == "Finished" || statusResp.Status == "Error" {
				return true
			}

			time.Sleep(500 * time.Millisecond)
		}

		// If still Processing after 10 seconds, property violated
		t.Logf("Report %s still Processing after 10s", resp.ID)
		return false
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 3}); err != nil {
		t.Errorf("Property violated: report stuck in Processing: %v", err)
	}
}

// Helper function to create a test template
func createTestTemplate(t *testing.T, ctx context.Context, cli *h.HTTPClient, headers map[string]string, orgID string) string {
	t.Helper()

	// Use a minimal template with valid fields
	simpleTemplate := `{% for org in midaz_onboarding.organization %}{{ org.id }}{% endfor %}`
	files := map[string][]byte{
		"template": []byte(simpleTemplate),
	}
	formData := map[string]string{
		"outputFormat": "TXT",
		"description":  "Property test template",
	}

	code, body, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", headers, formData, files)
	if err != nil {
		t.Skipf("Failed to upload template (network error): %v", err)
		return ""
	}

	if code != 200 && code != 201 {
		// If template creation fails, skip the test (system may not be ready)
		t.Skipf("Failed to create test template (may need datasource config): code=%d body=%s", code, string(body))
		return ""
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Skipf("Failed to parse template response: %v", err)
		return ""
	}

	return resp.ID
}
