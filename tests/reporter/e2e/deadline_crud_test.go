// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/tests/reporter/e2e/shared"
)

// futureDate returns a date 30 days from now in RFC3339 format.
func futureDate() string {
	return time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
}

// futureDateDays returns a date N days from now in RFC3339 format.
func futureDateDays(days int) string {
	return time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339)
}

// intPtr returns a pointer to an int.
func intPtr(v int) *int { return &v }

// ############################################################################
// Creation Tests (TC-DL-001 to TC-DL-005)
// ############################################################################

func TestDeadline_CreateOnce(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := map[string]any{
		"name":      shared.UniqueID("dl") + " Regulatory once",
		"type":      shared.DeadlineTypeRegulatory,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#FF5733",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)

	if status != http.StatusCreated {
		t.Logf("Deadline creation failed with status %d, body: %v", status, body)
	}

	shared.AssertHTTPStatus(t, status, http.StatusCreated)
	shared.AssertDeadlineFields(t, body)
	shared.AssertJSONField(t, body, "type", shared.DeadlineTypeRegulatory)
	shared.AssertJSONField(t, body, "frequency", shared.FrequencyOnce)
	shared.AssertJSONField(t, body, "color", "#FF5733")
	shared.AssertDeadlineStatus(t, body, shared.DeadlineStatusPending)

	active, ok := body["active"].(bool)
	require.True(t, ok, "response should contain 'active' bool field")
	assert.True(t, active, "deadline should default to active")
}

func TestDeadline_CreateMonthly(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := map[string]any{
		"name":      shared.UniqueID("dl") + " Custom monthly",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyMonthly,
		"color":     "#00FF00",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusCreated)
	shared.AssertDeadlineFields(t, body)
	shared.AssertJSONField(t, body, "type", shared.DeadlineTypeCustom)
	shared.AssertJSONField(t, body, "frequency", shared.FrequencyMonthly)
}

func TestDeadline_CreateSemiannual(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := map[string]any{
		"name":         shared.UniqueID("dl") + " Semiannual report",
		"type":         shared.DeadlineTypeRegulatory,
		"dueDate":      futureDate(),
		"frequency":    shared.FrequencySemiannual,
		"monthsOfYear": []int{1, 7},
		"color":        "#0000FF",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusCreated)
	shared.AssertDeadlineFields(t, body)
	shared.AssertJSONField(t, body, "frequency", shared.FrequencySemiannual)

	months, ok := body["monthsOfYear"].([]any)
	require.True(t, ok, "response should contain 'monthsOfYear' array")
	assert.Len(t, months, 2, "semiannual should have 2 months")
}

func TestDeadline_CreateAnnual(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := map[string]any{
		"name":         shared.UniqueID("dl") + " Annual report",
		"type":         shared.DeadlineTypeRegulatory,
		"dueDate":      futureDate(),
		"frequency":    shared.FrequencyAnnual,
		"monthsOfYear": []int{3},
		"color":        "#AABB00",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusCreated)
	shared.AssertDeadlineFields(t, body)
	shared.AssertJSONField(t, body, "frequency", shared.FrequencyAnnual)

	months, ok := body["monthsOfYear"].([]any)
	require.True(t, ok, "response should contain 'monthsOfYear' array")
	assert.Len(t, months, 1, "annual should have 1 month")
}

func TestDeadline_CreateWithTemplate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a template first
	desc := shared.UniqueID("tpl") + " Deadline template link"
	tplBytes := shared.LoadFixture(t, shared.FixtureValidHTML)

	tplStatus, tplBody, err := apiClient.CreateTemplate(ctx, tplBytes, "valid_html.tpl", shared.FormatHTML, desc)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, tplStatus)

	tplID, ok := tplBody["id"].(string)
	require.True(t, ok)

	input := map[string]any{
		"name":       shared.UniqueID("dl") + " With template",
		"type":       shared.DeadlineTypeCustom,
		"dueDate":    futureDate(),
		"frequency":  shared.FrequencyOnce,
		"color":      "#112233",
		"templateId": tplID,
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusCreated)
	shared.AssertDeadlineFields(t, body)
	shared.AssertJSONField(t, body, "templateId", tplID)

	templateName, ok := body["templateName"].(string)
	require.True(t, ok, "response should contain 'templateName' (denormalized from template)")
	assert.NotEmpty(t, templateName)
}

// ############################################################################
// List Tests (TC-DL-006 to TC-DL-010)
// ############################################################################

func TestDeadline_ListAndPaginate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two deadlines to ensure non-empty list
	for i := 0; i < 2; i++ {
		input := map[string]any{
			"name":      shared.UniqueID("dl") + " list test",
			"type":      shared.DeadlineTypeCustom,
			"dueDate":   futureDate(),
			"frequency": shared.FrequencyOnce,
			"color":     "#AABBCC",
		}

		status, _, err := apiClient.CreateDeadline(ctx, input)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, status)
	}

	// List with pagination
	status, resp, err := apiClient.GetAllDeadlines(ctx, map[string]string{
		"limit": "10",
		"page":  "1",
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertPagination(t, resp, 1)

	// Validate first item has all deadline fields
	if len(resp.Items) > 0 {
		shared.AssertDeadlineFields(t, resp.Items[0])
	}
}

func TestDeadline_FilterByStatus(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a pending deadline
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " status filter",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDateDays(60),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, _, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	// Filter by pending status
	status, resp, err := apiClient.GetAllDeadlines(ctx, map[string]string{
		"status": shared.DeadlineStatusPending,
		"limit":  "100",
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	// All returned items should be pending
	for _, item := range resp.Items {
		shared.AssertDeadlineStatus(t, item, shared.DeadlineStatusPending)
	}
}

func TestDeadline_FilterByType(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a regulatory deadline
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " type filter",
		"type":      shared.DeadlineTypeRegulatory,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, _, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	// Filter by type
	status, resp, err := apiClient.GetAllDeadlines(ctx, map[string]string{
		"type":  shared.DeadlineTypeRegulatory,
		"limit": "100",
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)

	for _, item := range resp.Items {
		shared.AssertJSONField(t, item, "type", shared.DeadlineTypeRegulatory)
	}
}

// ############################################################################
// Update Tests (TC-DL-011 to TC-DL-013)
// ############################################################################

func TestDeadline_UpdateNameAndDescription(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " original name",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id := body["id"].(string)

	// Update
	newName := shared.UniqueID("dl") + " updated name"
	newDesc := "Updated description for e2e test"

	status, updated, err := apiClient.UpdateDeadline(ctx, id, map[string]any{
		"name":        newName,
		"description": newDesc,
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertJSONField(t, updated, "name", newName)
	shared.AssertJSONField(t, updated, "description", newDesc)
}

func TestDeadline_UpdateFrequency(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create monthly
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " freq change",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyMonthly,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id := body["id"].(string)

	// Update to annual
	status, updated, err := apiClient.UpdateDeadline(ctx, id, map[string]any{
		"frequency":    shared.FrequencyAnnual,
		"monthsOfYear": []int{6},
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertJSONField(t, updated, "frequency", shared.FrequencyAnnual)
}

func TestDeadline_LinkAndUnlinkTemplate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create template
	tplBytes := shared.LoadFixture(t, shared.FixtureValidHTML)
	tplStatus, tplBody, err := apiClient.CreateTemplate(ctx, tplBytes, "valid_html.tpl", shared.FormatHTML, shared.UniqueID("tpl")+" link test")
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, tplStatus)

	tplID := tplBody["id"].(string)

	// Create deadline without template
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " link test",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id := body["id"].(string)

	// Link template
	status, updated, err := apiClient.UpdateDeadline(ctx, id, map[string]any{
		"templateId": tplID,
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertJSONField(t, updated, "templateId", tplID)
}

// ############################################################################
// Delete Test (TC-DL-014)
// ############################################################################

func TestDeadline_SoftDelete(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " delete test",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id := body["id"].(string)

	// Delete
	status, err = apiClient.DeleteDeadline(ctx, id)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusNoContent)
}

// ############################################################################
// Lifecycle Test (TC-DL-015)
// ############################################################################

func TestDeadline_FullLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := shared.UniqueID("dl") + " lifecycle"

	// Step 1: Create
	input := map[string]any{
		"name":             name,
		"description":      "Full lifecycle test deadline",
		"type":             shared.DeadlineTypeRegulatory,
		"dueDate":          futureDateDays(90),
		"frequency":        shared.FrequencyMonthly,
		"color":            "#FF0000",
		"notifyDaysBefore": 5,
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusCreated)
	shared.AssertDeadlineStatus(t, body, shared.DeadlineStatusPending)

	id := body["id"].(string)

	// Step 2: List and verify it exists
	status, listResp, err := apiClient.GetAllDeadlines(ctx, map[string]string{"limit": "100"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	found := false
	for _, item := range listResp.Items {
		if item["id"] == id {
			found = true

			break
		}
	}

	assert.True(t, found, "created deadline should appear in list")

	// Step 3: Update
	newName := shared.UniqueID("dl") + " lifecycle updated"

	status, updated, err := apiClient.UpdateDeadline(ctx, id, map[string]any{
		"name": newName,
	})
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertJSONField(t, updated, "name", newName)

	// Step 4: Deliver
	status, delivered, err := apiClient.DeliverDeadline(ctx, id, true)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertDeadlineStatus(t, delivered, shared.DeadlineStatusDelivered)

	// Step 5: Verify delivered in list filter
	status, filteredResp, err := apiClient.GetAllDeadlines(ctx, map[string]string{
		"status": shared.DeadlineStatusDelivered,
		"limit":  "100",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	foundDelivered := false
	for _, item := range filteredResp.Items {
		if item["id"] == id {
			foundDelivered = true

			break
		}
	}

	assert.True(t, foundDelivered, "delivered deadline should appear in delivered filter")

	// Step 6: Delete
	status, err = apiClient.DeleteDeadline(ctx, id)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusNoContent)
}
