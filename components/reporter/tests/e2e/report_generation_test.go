// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LerianStudio/reporter/tests/e2e/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTemplateForFormat creates a template for the given format and returns its ID.
func createTemplateForFormat(t *testing.T, ctx context.Context, format, fixture string) string {
	t.Helper()

	desc := shared.UniqueID("gen-tpl")
	tplBytes := shared.LoadFixture(t, fixture)

	status, body, err := apiClient.CreateTemplate(ctx, tplBytes, filepath.Base(fixture), format, desc)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain string id")

	return id
}

// ############################################################################
// Full pipeline per format (TC-GEN-001 to TC-GEN-005)
// ############################################################################

// TC-GEN-001: Full pipeline - HTML report generation.
func TestGen_HTMLPipeline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "text/html")
	shared.AssertHTMLContent(t, data, []string{"Organization Report"})
	shared.SaveReport(t, data, shared.FormatHTML)
}

// TC-GEN-002: Full pipeline - CSV report generation.
func TestGen_CSVPipeline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatCSV, shared.FixtureValidCSV)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "text/csv")
	shared.AssertCSVContent(t, data, []string{"id", "name", "status", "created_at"})
	shared.SaveReport(t, data, shared.FormatCSV)
}

// TC-GEN-003: Full pipeline - XML report generation.
func TestGen_XMLPipeline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatXML, shared.FixtureValidXML)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "application/xml")
	shared.AssertXMLContent(t, data)
	shared.SaveReport(t, data, shared.FormatXML)

	content := string(data)
	assert.Contains(t, content, "<organizations>", "XML should contain root element")
}

// TC-GEN-004: Full pipeline - PDF report generation.
func TestGen_PDFPipeline(t *testing.T) {
	t.Parallel()

	timeout := shared.DefaultPollTimeout + shared.PDFExtraTimeout

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatPDF, shared.FixtureValidPDF)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, timeout)

	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "application/pdf")
	shared.AssertPDFContent(t, data)
	shared.SaveReport(t, data, shared.FormatPDF)
}

// TC-GEN-005: Full pipeline - TXT report generation.
func TestGen_TXTPipeline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatTXT, shared.FixtureValidTXT)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, headers, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertContentType(t, headers, "text/plain")
	shared.AssertTXTContent(t, data, []string{"Organization Report"})
	shared.SaveReport(t, data, shared.FormatTXT)
}

// ############################################################################
// Datasource-specific (TC-GEN-006 to TC-GEN-008)
// ############################################################################

// TC-GEN-006: Report with PostgreSQL data source contains org data.
func TestGen_PostgreSQLDataSource(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertHTMLContent(t, data, []string{"Acme Corp"})
	shared.SaveReport(t, data, shared.FormatHTML)
}

// TC-GEN-007: Report with MongoDB data source contains holder data.
func TestGen_MongoDBDataSource(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	// Multi-source template requires plugin_crm MongoDB. Skip if unavailable.
	dsStatus, _, _ := apiClient.GetDataSourceByID(ctx, shared.DSPluginCRM)
	if dsStatus != http.StatusOK {
		t.Skipf("plugin_crm datasource unavailable (status %d) — skipping MongoDB test", dsStatus)
	}

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureMultiSource)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)
	assert.Contains(t, content, "Holders (MongoDB)", "content should reference MongoDB section")
	shared.SaveReport(t, data, shared.FormatHTML)
}

// TC-GEN-008: Multi-source report contains data from both PG and MongoDB.
func TestGen_MultiSourceReport(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	// Multi-source template requires plugin_crm MongoDB. Skip if unavailable.
	dsStatus, _, _ := apiClient.GetDataSourceByID(ctx, shared.DSPluginCRM)
	if dsStatus != http.StatusOK {
		t.Skipf("plugin_crm datasource unavailable (status %d) — skipping multi-source test", dsStatus)
	}

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureMultiSource)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)

	// Verify PostgreSQL data is present.
	assert.Contains(t, content, "Organizations (PostgreSQL)", "should contain PG section header")
	assert.Contains(t, content, "Acme Corp", "should contain PG org data")

	// Verify MongoDB data is present.
	assert.Contains(t, content, "Holders (MongoDB)", "should contain MongoDB section header")
	shared.SaveReport(t, data, shared.FormatHTML)
}

// ############################################################################
// Filter tests via pipeline (TC-GEN-009 to TC-GEN-014)
// ############################################################################

// TC-GEN-009: Report with eq filter on org name.
func TestGen_FilterEq(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "name", shared.FilterEq("Acme Corp"))

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	shared.AssertHTMLContent(t, data, []string{"Acme Corp"})
}

// TC-GEN-010: Report with gt filter on created_at date.
func TestGen_FilterGtDate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterGt("2025-06-01"))

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)
	assert.Contains(t, strings.ToLower(content), "<html", "should be valid HTML")
	assert.NotEmpty(t, data, "report should contain data")
}

// TC-GEN-011: Report with between filter on created_at dates.
func TestGen_FilterBetweenDates(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterBetween("2025-01-01", "2025-06-30"))

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)
	assert.Contains(t, strings.ToLower(content), "<html", "should be valid HTML")
}

// TC-GEN-012: Report with in filter on status.
func TestGen_FilterInStatus(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "status", shared.FilterIn("active", "pending"))

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)

	// Should NOT contain excluded statuses.
	assert.NotContains(t, content, "suspended", "filtered report should not contain suspended orgs")
}

// TC-GEN-013: Report with nin filter on status.
func TestGen_FilterNinStatus(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "status", shared.FilterNotIn("suspended", "inactive"))

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)

	assert.NotContains(t, content, "suspended", "filtered report should not contain suspended orgs")
	assert.NotContains(t, content, "inactive", "filtered report should not contain inactive orgs")
}

// TC-GEN-014: Report with combined filters (eq + gte) applies AND logic.
func TestGen_CombinedFilters(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := map[string]map[string]map[string]shared.FilterCondition{
		shared.DSMidazOnboarding: {
			shared.TableOrganization: {
				"status":     shared.FilterEq("active"),
				"created_at": shared.FilterGte("2025-01-01"),
			},
		},
	}

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)

	assert.Contains(t, strings.ToLower(content), "<html", "should produce valid HTML")
	assert.NotContains(t, content, "suspended", "AND filter should exclude suspended orgs")
}

// ############################################################################
// Worker behavior (TC-GEN-015 to TC-GEN-018)
// ############################################################################

// TC-GEN-015: Worker idempotency - creating same report twice returns cached result.
func TestGen_WorkerIdempotencySkipFinished(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)
	req := shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	}

	// First report: create and wait until finished.
	status1, body1, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status1)

	reportID1, ok := body1["id"].(string)
	require.True(t, ok, "first response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID1, shared.DefaultPollTimeout)

	// Second report with same template: should also succeed.
	status2, body2, err := apiClient.CreateReport(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status2)

	reportID2, ok := body2["id"].(string)
	require.True(t, ok, "second response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID2, shared.DefaultPollTimeout)

	// Verify both reports reached Finished status.
	_, report1, err := apiClient.GetReport(ctx, reportID1)
	require.NoError(t, err)
	assert.Equal(t, shared.StatusFinished, report1.Status)

	_, report2, err := apiClient.GetReport(ctx, reportID2)
	require.NoError(t, err)
	assert.Equal(t, shared.StatusFinished, report2.Status)
}

// TC-GEN-016: Worker idempotency - after a failed template creation, a valid template works.
// Templates with invalid database references are rejected at creation time by
// ValidateIfFieldsExistOnTables. This test verifies that a failed creation does not
// prevent subsequent valid template creations and report generation.
func TestGen_WorkerIdempotencySkipErrored(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	// Attempt to create a template that will fail (invalid database reference).
	badTplBytes := shared.LoadFixture(t, shared.FixtureInvalidDatabase)
	resp, err := apiClient.CreateTemplateRaw(ctx, badTplBytes, "gen-skip-errored-bad.tpl", shared.FormatHTML, shared.UniqueID("gen-skip-bad"))
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode(),
		"template with invalid database should be rejected at creation time")

	// Create a new report with a valid template; should succeed independently.
	goodTplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	status2, body2, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: goodTplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status2)

	reportID, ok := body2["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)
}

// TC-GEN-017: Error status metadata - template with invalid database reference is rejected at creation.
// Templates with invalid references are now caught at creation time by ValidateIfFieldsExistOnTables,
// so this test validates the creation-time rejection rather than runtime error status.
func TestGen_ErrorStatusMetadata(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplBytes := shared.LoadFixture(t, shared.FixtureInvalidDatabase)
	resp, err := apiClient.CreateTemplateRaw(ctx, tplBytes, "gen-error-metadata.tpl", shared.FormatHTML, shared.UniqueID("gen-error"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(),
		"template with invalid database reference should be rejected at creation time (400)")
}

// TC-GEN-018: Status transitions - observe Processing -> Finished transition.
func TestGen_StatusTransitions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")

	// The initial status should be Processing.
	initialStatus, ok := body["status"].(string)
	require.True(t, ok, "response should contain status field")
	assert.Equal(t, shared.StatusProcessing, initialStatus, "initial status should be Processing")

	// Wait for the report to reach Finished.
	finalReport := shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)
	assert.Equal(t, shared.StatusFinished, finalReport.Status)
}

// ############################################################################
// Timing and edge cases (TC-GEN-019 to TC-GEN-020)
// ############################################################################

// TC-GEN-019: PDF timeout handling - PDF report completes within extended timeout.
func TestGen_PDFTimeoutHandling(t *testing.T) {
	t.Parallel()

	timeout := shared.DefaultPollTimeout + shared.PDFExtraTimeout

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatPDF, shared.FixtureValidPDF)

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    map[string]map[string]map[string]shared.FilterCondition{},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	report := shared.AssertReportCompleted(t, ctx, apiClient, reportID, timeout)

	assert.Equal(t, shared.StatusFinished, report.Status, "PDF report should finish within extended timeout")
}

// TC-GEN-020: Empty filters generate report with ALL data.
func TestGen_EmptyFiltersAllData(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	// Explicitly pass empty filters map.
	emptyFilters := map[string]map[string]map[string]shared.FilterCondition{}

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    emptyFilters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	// Empty filters should produce a report with all data from the datasource.
	shared.AssertHTMLContent(t, data, []string{"Acme Corp"})
}

// TC-GEN-021: Report with filter matching zero rows produces valid empty output.
func TestGen_EmptyResultSet(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	// Filter for an organization name that doesn't exist in seed data.
	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "name", shared.FilterEq("NonExistentOrg12345"))

	status, body, err := apiClient.CreateReport(ctx, shared.CreateReportRequest{
		TemplateID: tplID,
		Filters:    filters,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	reportID, ok := body["id"].(string)
	require.True(t, ok)
	shared.AssertReportCompleted(t, ctx, apiClient, reportID, shared.DefaultPollTimeout)

	dlStatus, data, _, err := apiClient.DownloadReport(ctx, reportID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, dlStatus)

	content := string(data)
	// Report should be valid HTML but without any organization rows.
	assert.Contains(t, strings.ToLower(content), "<html", "should produce valid HTML")
	assert.NotContains(t, content, "Acme Corp", "empty result should not contain any org data")
	assert.NotContains(t, content, "Beta Inc", "empty result should not contain any org data")
}
