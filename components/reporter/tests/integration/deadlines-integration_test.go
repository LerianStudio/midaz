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

	h "github.com/LerianStudio/midaz/v3/components/reporter/tests/utils"
)

// deadlineResponse mirrors the deadline JSON returned by the API.
type deadlineResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Description      string  `json:"description,omitempty"`
	Type             string  `json:"type"`
	TemplateID       *string `json:"templateId,omitempty"`
	TemplateName     string  `json:"templateName,omitempty"`
	DueDate          string  `json:"dueDate"`
	Frequency        string  `json:"frequency"`
	MonthsOfYear     []int   `json:"monthsOfYear,omitempty"`
	Active           bool    `json:"active"`
	NotifyDaysBefore int     `json:"notifyDaysBefore,omitempty"`
	Color            string  `json:"color"`
	Status           string  `json:"status"`
	DeliveredAt      *string `json:"deliveredAt,omitempty"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
}

// deadlineListResponse mirrors the paginated list response.
type deadlineListResponse struct {
	Items []deadlineResponse `json:"items"`
	Limit int                `json:"limit"`
	Page  int                `json:"page"`
}

// createDeadlinePayload builds a valid deadline creation payload with a future dueDate.
func createDeadlinePayload(name string) map[string]any {
	return map[string]any{
		"name":             name,
		"description":      "Integration test deadline",
		"type":             "regulatory",
		"dueDate":          time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339),
		"frequency":        "monthly",
		"active":           true,
		"notifyDaysBefore": 5,
		"color":            "#FF5733",
	}
}

// createDeadlineViaAPI creates a deadline through the Manager API and returns the parsed response.
func createDeadlineViaAPI(t *testing.T, cli *h.HTTPClient, headers map[string]string, payload map[string]any) deadlineResponse {
	t.Helper()

	ctx := context.Background()

	code, body, err := cli.Request(ctx, "POST", "/v1/deadlines", headers, payload)
	if err != nil {
		t.Fatalf("POST /v1/deadlines request error: %v", err)
	}

	if code != 201 {
		t.Fatalf("POST /v1/deadlines expected 201, got %d body=%s", code, string(body))
	}

	var dl deadlineResponse
	if err := json.Unmarshal(body, &dl); err != nil {
		t.Fatalf("failed to unmarshal deadline response: %v", err)
	}

	if dl.ID == "" {
		t.Fatal("created deadline has empty ID")
	}

	return dl
}

// TestIntegration_Deadline_CreateAndRetrieve verifies IS-1: create a deadline via the API
// and retrieve it by listing deadlines.
func TestIntegration_Deadline_CreateAndRetrieve(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	uniqueName := "IS1-CreateRetrieve-" + h.RandString(8)
	payload := createDeadlinePayload(uniqueName)

	created := createDeadlineViaAPI(t, cli, headers, payload)

	// Verify the created deadline has correct fields
	if created.Name != uniqueName {
		t.Fatalf("expected name %q, got %q", uniqueName, created.Name)
	}

	if created.Type != "regulatory" {
		t.Fatalf("expected type 'regulatory', got %q", created.Type)
	}

	if created.Frequency != "monthly" {
		t.Fatalf("expected frequency 'monthly', got %q", created.Frequency)
	}

	if created.Color != "#FF5733" {
		t.Fatalf("expected color '#FF5733', got %q", created.Color)
	}

	if !created.Active {
		t.Fatal("expected active=true, got false")
	}

	// Retrieve by listing and filtering to find the created deadline
	listPath := fmt.Sprintf("/v1/deadlines?limit=50&page=1")
	code, body, err := cli.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET /v1/deadlines code=%d err=%v body=%s", code, err, string(body))
	}

	var listResp deadlineListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("failed to unmarshal list response: %v", err)
	}

	found := false
	for _, dl := range listResp.Items {
		if dl.ID == created.ID {
			found = true

			if dl.Name != uniqueName {
				t.Fatalf("retrieved deadline name mismatch: expected %q, got %q", uniqueName, dl.Name)
			}

			break
		}
	}

	if !found {
		t.Fatalf("created deadline %s not found in list response (%d items)", created.ID, len(listResp.Items))
	}
}

// TestIntegration_Deadline_ListWithPagination verifies IS-2: listing deadlines with pagination
// returns non-overlapping pages.
func TestIntegration_Deadline_ListWithPagination(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Create enough deadlines for pagination (at least 3)
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("IS2-Pagination-%d-%s", i, h.RandString(6))
		payload := createDeadlinePayload(name)
		createDeadlineViaAPI(t, cli, headers, payload)
	}

	// Fetch page 1 with limit=2
	code1, body1, err := cli.Request(ctx, "GET", "/v1/deadlines?limit=2&page=1", headers, nil)
	if err != nil || code1 != 200 {
		t.Fatalf("list page1 code=%d err=%v body=%s", code1, err, string(body1))
	}

	var page1 deadlineListResponse
	if err := json.Unmarshal(body1, &page1); err != nil {
		t.Fatalf("failed to unmarshal page1: %v", err)
	}

	// Fetch page 2 with limit=2
	code2, body2, err := cli.Request(ctx, "GET", "/v1/deadlines?limit=2&page=2", headers, nil)
	if err != nil || code2 != 200 {
		t.Fatalf("list page2 code=%d err=%v body=%s", code2, err, string(body2))
	}

	var page2 deadlineListResponse
	if err := json.Unmarshal(body2, &page2); err != nil {
		t.Fatalf("failed to unmarshal page2: %v", err)
	}

	// Verify page 1 has at most 2 items
	if len(page1.Items) > 2 {
		t.Fatalf("page1 returned more than limit=2 items: got %d", len(page1.Items))
	}

	// Verify no overlap between pages
	page1IDs := make(map[string]bool)
	for _, dl := range page1.Items {
		page1IDs[dl.ID] = true
	}

	for _, dl := range page2.Items {
		if page1IDs[dl.ID] {
			t.Fatalf("duplicate deadline %s found across page 1 and page 2", dl.ID)
		}
	}
}

// TestIntegration_Deadline_ListWithFilters verifies IS-3: listing deadlines with query filters.
func TestIntegration_Deadline_ListWithFilters(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Create a deadline we can filter for
	uniqueName := "IS3-Filter-" + h.RandString(8)
	payload := createDeadlinePayload(uniqueName)
	payload["type"] = "custom"
	payload["active"] = true

	created := createDeadlineViaAPI(t, cli, headers, payload)

	// Filter by status — all newly created future deadlines should be "pending"
	code, body, err := cli.Request(ctx, "GET", "/v1/deadlines?limit=50&page=1&status=pending", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET deadlines with status filter code=%d err=%v body=%s", code, err, string(body))
	}

	var filtered deadlineListResponse
	if err := json.Unmarshal(body, &filtered); err != nil {
		t.Fatalf("failed to unmarshal filtered response: %v", err)
	}

	if len(filtered.Items) == 0 {
		t.Fatal("expected at least one deadline with status=pending, got 0")
	}

	// Every returned item must match the requested status
	for _, item := range filtered.Items {
		if item.Status != "pending" {
			t.Fatalf("status filter returned item %s with status %q, expected \"pending\"", item.ID, item.Status)
		}
	}

	// The deadline we just created must be present in the filtered results
	found := false
	for _, item := range filtered.Items {
		if item.ID == created.ID {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("created deadline %s not found in status=pending filtered results", created.ID)
	}

	// Verify pagination parameters are respected
	codeLimited, bodyLimited, err := cli.Request(ctx, "GET", "/v1/deadlines?limit=1&page=1", headers, nil)
	if err != nil || codeLimited != 200 {
		t.Fatalf("GET deadlines with limit=1 code=%d err=%v body=%s", codeLimited, err, string(bodyLimited))
	}

	var limitedResp deadlineListResponse
	if err := json.Unmarshal(bodyLimited, &limitedResp); err != nil {
		t.Fatalf("failed to unmarshal limited response: %v", err)
	}

	if len(limitedResp.Items) > 1 {
		t.Fatalf("expected at most 1 item with limit=1, got %d", len(limitedResp.Items))
	}
}

// TestIntegration_Deadline_UpdateFields verifies IS-4: updating deadline fields via PUT.
func TestIntegration_Deadline_UpdateFields(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Create a deadline to update
	uniqueName := "IS4-Update-" + h.RandString(8)
	payload := createDeadlinePayload(uniqueName)
	created := createDeadlineViaAPI(t, cli, headers, payload)

	// Update the name and description
	updatedName := "IS4-Updated-" + h.RandString(8)
	updatedDesc := "Updated description for integration test"
	updatedColor := "#00FF00"
	updatePayload := map[string]any{
		"name":        updatedName,
		"description": updatedDesc,
		"color":       updatedColor,
	}

	updatePath := fmt.Sprintf("/v1/deadlines/%s", created.ID)
	code, body, err := cli.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil {
		t.Fatalf("PUT %s request error: %v", updatePath, err)
	}

	if code != 200 {
		t.Fatalf("PUT %s expected 200, got %d body=%s", updatePath, code, string(body))
	}

	var updated deadlineResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		t.Fatalf("failed to unmarshal update response: %v", err)
	}

	if updated.Name != updatedName {
		t.Fatalf("expected updated name %q, got %q", updatedName, updated.Name)
	}

	if updated.Description != updatedDesc {
		t.Fatalf("expected updated description %q, got %q", updatedDesc, updated.Description)
	}

	if updated.Color != updatedColor {
		t.Fatalf("expected updated color %q, got %q", updatedColor, updated.Color)
	}

	// Verify the ID did not change
	if updated.ID != created.ID {
		t.Fatalf("deadline ID changed after update: was %q, now %q", created.ID, updated.ID)
	}
}

// TestIntegration_Deadline_SoftDelete verifies IS-5: soft deleting a deadline sets deleted_at
// and excludes it from FindList results.
func TestIntegration_Deadline_SoftDelete(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Create a deadline to delete
	uniqueName := "IS5-SoftDelete-" + h.RandString(8)
	payload := createDeadlinePayload(uniqueName)
	created := createDeadlineViaAPI(t, cli, headers, payload)

	// Verify it appears in the list
	listPath := "/v1/deadlines?limit=50&page=1"
	code, body, err := cli.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET list before delete code=%d err=%v body=%s", code, err, string(body))
	}

	var beforeDelete deadlineListResponse
	if err := json.Unmarshal(body, &beforeDelete); err != nil {
		t.Fatalf("failed to unmarshal before-delete list: %v", err)
	}

	foundBefore := false
	for _, dl := range beforeDelete.Items {
		if dl.ID == created.ID {
			foundBefore = true
			break
		}
	}

	if !foundBefore {
		t.Fatalf("created deadline %s not found in list before delete", created.ID)
	}

	// Soft delete the deadline
	deletePath := fmt.Sprintf("/v1/deadlines/%s", created.ID)
	codeDelete, bodyDelete, err := cli.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil {
		t.Fatalf("DELETE %s request error: %v", deletePath, err)
	}

	if codeDelete != 204 {
		t.Fatalf("DELETE %s expected 204, got %d body=%s", deletePath, codeDelete, string(bodyDelete))
	}

	// Verify it no longer appears in the list
	code, body, err = cli.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET list after delete code=%d err=%v body=%s", code, err, string(body))
	}

	var afterDelete deadlineListResponse
	if err := json.Unmarshal(body, &afterDelete); err != nil {
		t.Fatalf("failed to unmarshal after-delete list: %v", err)
	}

	for _, dl := range afterDelete.Items {
		if dl.ID == created.ID {
			t.Fatalf("soft-deleted deadline %s still appears in list", created.ID)
		}
	}
}

// TestIntegration_Deadline_DeliverAndStatusComputation verifies IS-6: delivering a deadline
// sets delivered_at and the status is computed as "delivered".
func TestIntegration_Deadline_DeliverAndStatusComputation(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Create a deadline with a future due date (status should be "pending")
	uniqueName := "IS6-Deliver-" + h.RandString(8)
	payload := createDeadlinePayload(uniqueName)
	created := createDeadlineViaAPI(t, cli, headers, payload)

	if created.Status != "pending" {
		t.Fatalf("expected initial status 'pending', got %q", created.Status)
	}

	if created.DeliveredAt != nil {
		t.Fatal("expected deliveredAt to be nil before delivery")
	}

	// Deliver the deadline
	deliverPath := fmt.Sprintf("/v1/deadlines/%s/deliver", created.ID)
	deliverPayload := map[string]any{"delivered": true}

	code, body, err := cli.Request(ctx, "PATCH", deliverPath, headers, deliverPayload)
	if err != nil {
		t.Fatalf("PATCH %s request error: %v", deliverPath, err)
	}

	if code != 200 {
		t.Fatalf("PATCH %s expected 200, got %d body=%s", deliverPath, code, string(body))
	}

	var delivered deadlineResponse
	if err := json.Unmarshal(body, &delivered); err != nil {
		t.Fatalf("failed to unmarshal deliver response: %v", err)
	}

	if delivered.Status != "delivered" {
		t.Fatalf("expected status 'delivered' after delivery, got %q", delivered.Status)
	}

	if delivered.DeliveredAt == nil {
		t.Fatal("expected deliveredAt to be non-nil after delivery")
	}

	// Undeliver (clear delivered_at) and verify status reverts
	undeliverPayload := map[string]any{"delivered": false}
	code, body, err = cli.Request(ctx, "PATCH", deliverPath, headers, undeliverPayload)
	if err != nil {
		t.Fatalf("PATCH (undeliver) %s request error: %v", deliverPath, err)
	}

	if code != 200 {
		t.Fatalf("PATCH (undeliver) %s expected 200, got %d body=%s", deliverPath, code, string(body))
	}

	var undelivered deadlineResponse
	if err := json.Unmarshal(body, &undelivered); err != nil {
		t.Fatalf("failed to unmarshal undeliver response: %v", err)
	}

	if undelivered.Status == "delivered" {
		t.Fatal("expected status to revert from 'delivered' after undelivery")
	}

	if undelivered.DeliveredAt != nil {
		t.Fatal("expected deliveredAt to be nil after undelivery")
	}
}

// TestIntegration_Deadline_CreateWithTemplateID verifies IS-7: creating a deadline with a
// templateId resolves the templateName from the template collection.
func TestIntegration_Deadline_CreateWithTemplateID(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Step 1: Create a template to reference
	templateDesc := "IS7-Template-" + h.RandString(8)
	templateContent := []byte("<html><body>{{ title }}</body></html>")

	formData := map[string]string{
		"description":  templateDesc,
		"outputFormat": "HTML",
	}
	files := map[string][]byte{
		"template": templateContent,
	}

	tplCode, tplBody, err := cli.UploadMultipartForm(ctx, "POST", "/v1/templates", h.AuthHeaders(), formData, files)
	if err != nil {
		t.Fatalf("POST /v1/templates request error: %v", err)
	}

	if tplCode != 201 {
		t.Fatalf("POST /v1/templates expected 201, got %d body=%s", tplCode, string(tplBody))
	}

	var tplResp struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal(tplBody, &tplResp); err != nil {
		t.Fatalf("failed to unmarshal template response: %v", err)
	}

	if tplResp.ID == "" {
		t.Fatal("template creation returned empty ID")
	}

	// Step 2: Create a deadline referencing the template
	uniqueName := "IS7-WithTemplate-" + h.RandString(8)
	deadlinePayload := createDeadlinePayload(uniqueName)
	deadlinePayload["templateId"] = tplResp.ID

	created := createDeadlineViaAPI(t, cli, headers, deadlinePayload)

	// Step 3: Verify templateId and templateName are populated
	if created.TemplateID == nil || *created.TemplateID != tplResp.ID {
		t.Fatalf("expected templateId=%q, got %v", tplResp.ID, created.TemplateID)
	}

	// The service resolves templateName from the template's description
	if created.TemplateName == "" {
		t.Fatal("expected templateName to be resolved from template, got empty string")
	}

	if created.TemplateName != templateDesc {
		t.Fatalf("expected templateName=%q, got %q", templateDesc, created.TemplateName)
	}
}

// TestIntegration_Deadline_DeleteNonExistent verifies that deleting a non-existent deadline
// returns an appropriate error status code (404).
func TestIntegration_Deadline_DeleteNonExistent(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Attempt to delete a non-existent UUID
	fakePath := "/v1/deadlines/00000000-0000-0000-0000-000000000001"
	code, body, err := cli.Request(ctx, "DELETE", fakePath, headers, nil)
	if err != nil {
		t.Fatalf("DELETE %s request error: %v", fakePath, err)
	}

	// Expect 404 (entity not found) — the exact code depends on the error mapping
	if code != 404 {
		t.Fatalf("DELETE non-existent deadline expected 404, got %d body=%s", code, string(body))
	}
}

// TestIntegration_Deadline_UpdateNonExistent verifies that updating a non-existent deadline
// returns an appropriate error status code.
func TestIntegration_Deadline_UpdateNonExistent(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	fakePath := "/v1/deadlines/00000000-0000-0000-0000-000000000002"
	updatePayload := map[string]any{
		"name": "should-not-exist",
	}

	code, body, err := cli.Request(ctx, "PATCH", fakePath, headers, updatePayload)
	if err != nil {
		t.Fatalf("PATCH %s request error: %v", fakePath, err)
	}

	// Expect 404 (entity not found) — the H3 fix ensures Update with deleted_at guard returns not found
	if code != 404 {
		t.Fatalf("PATCH non-existent deadline expected 404, got %d body=%s", code, string(body))
	}
}

// TestIntegration_Deadline_DeliverNonExistent verifies that delivering a non-existent deadline
// returns an error.
func TestIntegration_Deadline_DeliverNonExistent(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	fakePath := "/v1/deadlines/00000000-0000-0000-0000-000000000003/deliver"
	deliverPayload := map[string]any{"delivered": true}

	code, body, err := cli.Request(ctx, "PATCH", fakePath, headers, deliverPayload)
	if err != nil {
		t.Fatalf("PATCH %s request error: %v", fakePath, err)
	}

	if code >= 200 && code < 300 {
		t.Fatalf("PATCH deliver non-existent deadline expected error, got %d body=%s", code, string(body))
	}
}

// TestIntegration_Deadline_CreateInvalidPayload verifies that creating a deadline with missing
// required fields returns a 400 Bad Request.
func TestIntegration_Deadline_CreateInvalidPayload(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Missing required fields (name, type, dueDate, frequency, color)
	invalidPayload := map[string]any{
		"description": "missing required fields",
	}

	code, body, err := cli.Request(ctx, "POST", "/v1/deadlines", headers, invalidPayload)
	if err != nil {
		t.Fatalf("POST /v1/deadlines (invalid) request error: %v", err)
	}

	if code < 400 || code >= 500 {
		t.Fatalf("expected 4xx for invalid payload, got %d body=%s", code, string(body))
	}
}

// TestIntegration_Deadline_DoubleDelete verifies that deleting an already-deleted deadline
// returns 404 (since it has deleted_at set and is excluded from lookups).
func TestIntegration_Deadline_DoubleDelete(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	// Create and then delete a deadline
	uniqueName := "IS5-DoubleDelete-" + h.RandString(8)
	payload := createDeadlinePayload(uniqueName)
	created := createDeadlineViaAPI(t, cli, headers, payload)

	deletePath := fmt.Sprintf("/v1/deadlines/%s", created.ID)

	code, body, err := cli.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil {
		t.Fatalf("first DELETE request error: %v", err)
	}

	if code != 204 {
		t.Fatalf("first DELETE expected 204, got %d body=%s", code, string(body))
	}

	// Second delete should fail (entity already soft-deleted)
	code2, body2, err := cli.Request(ctx, "DELETE", deletePath, headers, nil)
	if err != nil {
		t.Fatalf("second DELETE request error: %v", err)
	}

	if code2 == 204 {
		t.Fatal("second DELETE should not return 204 for already-deleted deadline")
	}

	if code2 != 404 {
		t.Fatalf("second DELETE expected 404 for already-deleted deadline, got %d body=%s", code2, string(body2))
	}
}
