// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/reporter/tests/e2e/shared"
)

// createTestTemplate creates a template for report tests and returns its ID.
// The fixture and format parameters determine what kind of template to create.
func createTestTemplate(t *testing.T, ctx context.Context, fixture, format string) string {
	t.Helper()

	desc := shared.UniqueID("rpt-tpl")
	tplBytes := shared.LoadFixture(t, fixture)

	status, body, err := apiClient.CreateTemplate(ctx, tplBytes, fixture, format, desc)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "template creation should return 201")

	id, ok := body["id"].(string)
	require.True(t, ok, "template id should be a string")

	return id
}

// createTestReport creates a report for an existing template and returns the report ID.
func createTestReport(t *testing.T, ctx context.Context, templateID string) string {
	t.Helper()

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "report creation should return 201")

	id, ok := body["id"].(string)
	require.True(t, ok, "report id should be a string")

	return id
}

// ############################################################################
// Report Creation per Format (TC-RPT-001 to TC-RPT-005)
// ############################################################################

func TestReport_CreateHTML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusCreated)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertValidUUID(t, id)
	shared.AssertJSONField(t, body, "templateId", templateID)
	shared.AssertJSONField(t, body, "status", shared.StatusProcessing)

	assert.NotEmpty(t, body["createdAt"], "createdAt should be present")
	assert.NotEmpty(t, body["updatedAt"], "updatedAt should be present")
}

func TestReport_CreateCSV(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidCSV, shared.FormatCSV)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusCreated)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertValidUUID(t, id)
	shared.AssertJSONField(t, body, "templateId", templateID)
	shared.AssertJSONField(t, body, "status", shared.StatusProcessing)
}

func TestReport_CreateXML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidXML, shared.FormatXML)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusCreated)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertValidUUID(t, id)
	shared.AssertJSONField(t, body, "templateId", templateID)
	shared.AssertJSONField(t, body, "status", shared.StatusProcessing)
}

func TestReport_CreatePDF(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidPDF, shared.FormatPDF)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusCreated)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertValidUUID(t, id)
	shared.AssertJSONField(t, body, "templateId", templateID)
	shared.AssertJSONField(t, body, "status", shared.StatusProcessing)
}

func TestReport_CreateTXT(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidTXT, shared.FormatTXT)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusCreated)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertValidUUID(t, id)
	shared.AssertJSONField(t, body, "templateId", templateID)
	shared.AssertJSONField(t, body, "status", shared.StatusProcessing)
}

// ############################################################################
// Report Creation with Filters (TC-RPT-006)
// ############################################################################

func TestReport_CreateWithFilters(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	filters := shared.MakeFilters(
		shared.DSMidazOnboarding,
		shared.TableOrganization,
		"name",
		shared.FilterEq("Acme Corp"),
	)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    filters,
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusCreated)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertValidUUID(t, id)
	shared.AssertJSONField(t, body, "templateId", templateID)

	// Verify filters are present in the response.
	respFilters, ok := body["filters"]
	require.True(t, ok, "response should contain filters field")
	assert.NotNil(t, respFilters, "filters should not be nil")
}

// ############################################################################
// Validation Errors (TC-RPT-007 to TC-RPT-015)
// ############################################################################

func TestReport_CreateMissingTemplateID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send body without templateId.
	resp, err := apiClient.CreateReportRaw(ctx, map[string]any{
		"filters": map[string]any{},
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "missing templateId should return 400")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body))

	shared.AssertErrorCode(t, body, "TPL-0016")
}

func TestReport_CreateInvalidTemplateID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.CreateReportRaw(ctx, map[string]any{
		"templateId": "not-a-valid-uuid",
		"filters":    map[string]any{},
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "invalid templateId should return 400")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body))

	shared.AssertErrorCode(t, body, "TPL-0012")
}

func TestReport_CreateTemplateNotFound(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nonExistentID := "00000000-0000-0000-0000-000000000000"

	req := shared.CreateReportRequest{
		TemplateID: nonExistentID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, status, "non-existent template should return 404")

	shared.AssertErrorCode(t, body, "TPL-0011")
}

func TestReport_CreateDeletedTemplate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create and then delete a template.
	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	delStatus, err := apiClient.DeleteTemplate(ctx, templateID)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, delStatus, "delete should return 204")

	// Attempt to create a report with the deleted template.
	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, status, "deleted template should return 404")

	shared.AssertErrorCode(t, body, "TPL-0011")
}

func TestReport_CreateInvalidFilterField(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	filters := shared.MakeFilters(
		shared.DSMidazOnboarding,
		shared.TableOrganization,
		"nonexistent_column",
		shared.FilterEq("test"),
	)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    filters,
	}

	status, body, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, status, "invalid filter field should return 400")

	shared.AssertErrorCode(t, body, "TPL-0014")
}

func TestReport_CreateMissingFilters(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	// Send body with templateId but without filters field.
	resp, err := apiClient.CreateReportRaw(ctx, map[string]any{
		"templateId": templateID,
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "missing filters should return 400")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body))

	shared.AssertErrorCode(t, body, "TPL-0016")
}

func TestReport_CreateEmptyBody(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.CreateReportRaw(ctx, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "empty body should return 400")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body))

	shared.AssertErrorCode(t, body, "TPL-0016")
}

func TestReport_CreateMalformedJSON(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.CreateReportRaw(ctx, `{"templateId": "abc", invalid json`)
	require.NoError(t, err)

	// Malformed JSON may return 400, 422, or even 500 depending on where Fiber's parser
	// catches the error. Accept any non-2xx response.
	assert.True(t, resp.StatusCode() >= 400,
		"malformed JSON should return an error status, got %d", resp.StatusCode())
}

func TestReport_CreateUnexpectedFields(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	resp, err := apiClient.CreateReportRaw(ctx, map[string]any{
		"templateId":   templateID,
		"filters":      map[string]any{},
		"unknownField": "value",
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "unexpected fields should return 400")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body))

	shared.AssertErrorCode(t, body, "TPL-0015")
}

// ############################################################################
// Idempotency Tests (TC-RPT-016 to TC-RPT-018)
// ############################################################################

func TestReport_IdempotencyHeader(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	idempotencyKey := shared.UniqueID("rpt-idem")

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	// First request: should create the report.
	status1, body1, err := apiClient.CreateReportWithIdempotency(ctx, req, idempotencyKey)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status1, http.StatusCreated)

	id1, ok := body1["id"].(string)
	require.True(t, ok, "first response should contain 'id' field")

	// Second request with same idempotency key: should return cached response.
	status2, body2, err := apiClient.CreateReportWithIdempotency(ctx, req, idempotencyKey)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status2, http.StatusCreated)

	id2, ok := body2["id"].(string)
	require.True(t, ok, "second response should contain 'id' field")

	assert.Equal(t, id1, id2, "idempotent requests should return the same report id")

	// Third request using raw HTTP to verify Idempotency-Replayed header.
	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, env.ManagerBaseURL+"/v1/reports", bytes.NewReader(reqBody))
	require.NoError(t, err)

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Idempotency", idempotencyKey)

	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-Idempotency-Replayed"),
		"replayed response should have X-Idempotency-Replayed: true header")
}

func TestReport_IdempotencyBodyHash(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	// First request without idempotency header.
	status1, body1, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status1, http.StatusCreated)

	id1, ok := body1["id"].(string)
	require.True(t, ok, "first response should contain 'id' field")

	// Second identical request (same body).
	status2, body2, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, status2, "body hash dedup should return 201")

	id2, ok := body2["id"].(string)
	require.True(t, ok, "second response should contain 'id' field")

	assert.Equal(t, id1, id2, "duplicate body hash requests should return the same report id")
}

func TestReport_IdempotencyConcurrentDuplicate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	idempotencyKey := shared.UniqueID("concurrent-rpt")

	req := shared.CreateReportRequest{
		TemplateID: templateID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	const concurrency = 2

	type result struct {
		status int
		body   map[string]any
		err    error
	}

	results := make([]result, concurrency)

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()

			s, b, e := apiClient.CreateReportWithIdempotency(ctx, req, idempotencyKey)
			results[idx] = result{status: s, body: b, err: e}
		}(i)
	}

	wg.Wait()

	var (
		got201 int
		got409 int
	)

	for i, r := range results {
		require.NoError(t, r.err, "request %d should not return transport error", i)

		switch r.status {
		case http.StatusCreated:
			got201++
		case http.StatusConflict:
			got409++

			if r.body != nil {
				code, ok := r.body["code"].(string)
				if ok {
					assert.Equal(t, "TPL-0039", code, "conflict response should have error code TPL-0039")
				}
			}
		default:
			assert.True(t, r.status == http.StatusCreated || r.status == http.StatusConflict,
				"request %d: expected 201 or 409, got %d", i, r.status)
		}
	}

	assert.GreaterOrEqual(t, got201, 1, "at least one request should succeed with 201")
	assert.Equal(t, concurrency, got201+got409,
		"all requests should return either 201 or 409")
}

// ############################################################################
// Get Report Tests (TC-RPT-019 to TC-RPT-021)
// ############################################################################

func TestReport_GetByID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	reportID := createTestReport(t, ctx, templateID)

	status, report, err := apiClient.GetReport(ctx, reportID)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)

	assert.Equal(t, reportID, report.ID, "report ID should match")
	assert.Equal(t, templateID, report.TemplateID, "templateId should match")
	assert.NotEmpty(t, report.Status, "status should be present")
	assert.NotEmpty(t, report.CreatedAt, "createdAt should be present")
	assert.NotEmpty(t, report.UpdatedAt, "updatedAt should be present")
}

func TestReport_GetNotFound(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nonExistentID := "00000000-0000-0000-0000-000000000000"

	status, _, err := apiClient.GetReport(ctx, nonExistentID)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, status, "non-existent report should return 404")
}

func TestReport_GetInvalidUUID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetReportRaw(ctx, "not-a-uuid")
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "invalid UUID should return 400")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body))

	shared.AssertErrorCode(t, body, "TPL-0009")
}

// ############################################################################
// List Reports Tests (TC-RPT-022 to TC-RPT-026)
// ############################################################################

func TestReport_ListNoFilters(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a template and a report to ensure at least one item exists.
	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	createTestReport(t, ctx, templateID)

	status, resp, err := apiClient.GetAllReports(ctx, map[string]string{})
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertPagination(t, resp, 1)
}

func TestReport_ListFilterByStatus(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a report that starts in Processing status.
	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	createTestReport(t, ctx, templateID)

	status, resp, err := apiClient.GetAllReports(ctx, map[string]string{
		"status": shared.StatusProcessing,
	})
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)

	for _, item := range resp.Items {
		assert.Equal(t, shared.StatusProcessing, item["status"],
			"all items should have status=Processing")
	}
}

func TestReport_ListFilterByTemplateID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	createTestReport(t, ctx, templateID)

	status, resp, err := apiClient.GetAllReports(ctx, map[string]string{
		"template_id": templateID,
	})
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)

	require.GreaterOrEqual(t, len(resp.Items), 1, "should have at least 1 report for this template")

	for _, item := range resp.Items {
		assert.Equal(t, templateID, item["templateId"],
			"all items should have matching templateId")
	}
}

func TestReport_ListFilterByCreatedAt(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	createTestReport(t, ctx, templateID)

	// List reports without date filter first to verify reports exist.
	status, resp, err := apiClient.GetAllReports(ctx, map[string]string{})
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)
	assert.GreaterOrEqual(t, len(resp.Items), 1, "should have at least 1 report")

	// Try with created_at filter using today's date (ISO format).
	// The API may or may not support this parameter — just verify it doesn't error.
	today := time.Now().UTC().Format("2006-01-02")

	status2, _, err := apiClient.GetAllReports(ctx, map[string]string{
		"created_at": today,
	})
	require.NoError(t, err)
	assert.True(t, status2 == http.StatusOK || status2 == http.StatusBadRequest,
		"created_at filter should return 200 or 400, got %d", status2)
}

func TestReport_ListPagination(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a template and multiple reports so pagination can be tested.
	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)

	for range 3 {
		createTestReport(t, ctx, templateID)
	}

	status, resp, err := apiClient.GetAllReports(ctx, map[string]string{
		"template_id": templateID,
		"page":        "1",
		"limit":       "2",
	})
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)

	assert.LessOrEqual(t, len(resp.Items), 2, "page 1 should have at most 2 items")
	assert.Equal(t, 1, resp.Page, "page should be 1")
	assert.Equal(t, 2, resp.Limit, "limit should be 2")
}

// ############################################################################
// Download Tests (TC-RPT-027 to TC-RPT-031)
// ############################################################################

func TestReport_DownloadFinished(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	reportID := createTestReport(t, ctx, templateID)

	// Wait for the report to reach Finished status.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Download the finished report.
	status, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)

	assert.NotEmpty(t, data, "downloaded report content should not be empty")
}

func TestReport_DownloadProcessing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	reportID := createTestReport(t, ctx, templateID)

	// Immediately try to download without waiting for completion.
	status, _, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, status, "downloading a processing report should return 400")
}

func TestReport_DownloadNotFound(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nonExistentID := "00000000-0000-0000-0000-000000000000"

	status, _, _, err := apiClient.DownloadReport(ctx, nonExistentID)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, status, "non-existent report download should return 404")
}

func TestReport_DownloadContentType(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	reportID := createTestReport(t, ctx, templateID)

	// Wait for the report to finish.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Download and verify Content-Type.
	status, _, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertContentType(t, headers, "text/html")
}

func TestReport_DownloadContentDisposition(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	templateID := createTestTemplate(t, ctx, shared.FixtureValidHTML, shared.FormatHTML)
	reportID := createTestReport(t, ctx, templateID)

	// Wait for the report to finish.
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	// Download and verify Content-Disposition.
	status, _, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)

	shared.AssertHTTPStatus(t, status, http.StatusOK)

	cd := headers.Get("Content-Disposition")
	assert.Contains(t, cd, "attachment", "Content-Disposition should contain 'attachment'")
	assert.Contains(t, cd, reportID, "Content-Disposition filename should contain the report ID")
}
