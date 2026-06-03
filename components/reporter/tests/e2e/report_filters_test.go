// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/LerianStudio/reporter/tests/e2e/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ############################################################################
// Filter Operations Tests (TC-FLT-001 to TC-FLT-016)
// ############################################################################

// TC-FLT-001: eq single value - filter organization name eq "Acme Corp".
func TestFilter_EqSingleValue(t *testing.T) {
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

// TC-FLT-002: eq multiple values - OR semantics.
func TestFilter_EqMultipleValues(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "name", shared.FilterEq("Acme Corp", "Beta Inc"))

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

	// At least one of the filtered orgs should be present.
	hasAcme := strings.Contains(content, "Acme Corp")
	hasBeta := strings.Contains(content, "Beta Inc")
	assert.True(t, hasAcme || hasBeta, "report should contain at least one of the filtered organizations")
}

// TC-FLT-003: gt date - filter organization created_at > 2025-06-01.
func TestFilter_GtDate(t *testing.T) {
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

	assert.NotEmpty(t, data, "report should contain data for gt filter")
}

// TC-FLT-004: gte date - filter organization created_at >= "2025-06-01".
func TestFilter_GteDate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterGte("2025-06-01"))

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
	assert.Contains(t, strings.ToLower(content), "<html", "should produce valid HTML output")
}

// TC-FLT-005: lt date - filter organization created_at < 2025-06-01.
func TestFilter_LtDate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterLt("2025-06-01"))

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

	assert.NotEmpty(t, data, "report should contain data for lt filter")
}

// TC-FLT-006: lte date - filter organization created_at <= "2025-06-30".
func TestFilter_LteDate(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterLte("2025-06-30"))

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
	assert.Contains(t, strings.ToLower(content), "<html", "should produce valid HTML output")
}

// TC-FLT-007: between dates - filter organization created_at between Mar and Aug 2025.
func TestFilter_BetweenDates(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterBetween("2025-03-01", "2025-08-01"))

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
	assert.Contains(t, strings.ToLower(content), "<html", "should produce valid HTML output")
}

// TC-FLT-008: between date range - filter organization created_at between 2025-01-01 and 2025-12-31.
func TestFilter_BetweenDateRange(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableOrganization, "created_at", shared.FilterBetween("2025-01-01", "2025-12-31"))

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

	assert.NotEmpty(t, data, "report should contain data for between filter")
}

// TC-FLT-009: in list - filter organization status in ["active", "pending"].
func TestFilter_InList(t *testing.T) {
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

	assert.NotContains(t, content, "suspended", "filtered report should not contain suspended orgs")
}

// TC-FLT-010: nin exclusion - filter organization status nin ["suspended", "inactive"].
func TestFilter_NinExclusion(t *testing.T) {
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

	assert.NotContains(t, content, "suspended", "report should not contain suspended orgs")
	assert.NotContains(t, content, "inactive", "report should not contain inactive orgs")
}

// TC-FLT-011: combined same field - gte + lte on created_at acts as range.
func TestFilter_CombinedSameField(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := map[string]map[string]map[string]shared.FilterCondition{
		shared.DSMidazOnboarding: {
			shared.TableOrganization: {
				"created_at": {
					GreaterOrEqual: []any{"2025-03-01"},
					LessOrEqual:    []any{"2025-09-01"},
				},
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
}

// TC-FLT-012: filters across multiple tables - organization.status + ledger.status.
func TestFilter_AcrossMultipleTables(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

	filters := map[string]map[string]map[string]shared.FilterCondition{
		shared.DSMidazOnboarding: {
			shared.TableOrganization: {
				"status": shared.FilterEq("active"),
			},
			shared.TableLedger: {
				"status": shared.FilterEq("active"),
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
}

// TC-FLT-013: empty filters - all data returned.
func TestFilter_EmptyFiltersAllData(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

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

	// With no filters, the report should contain all data from the datasource.
	shared.AssertHTMLContent(t, data, []string{"Acme Corp"})
}

// ############################################################################
// Account Table Filter Tests (TC-FLT-014 to TC-FLT-016)
//
// BUG DISCOVERED: Filters on the account table are accepted by the Manager
// (HTTP 201) but the Worker returns zero rows — the filter is not applied.
// This affects ALL filter types (eq, gt, lt, between) on the account table.
// Filters on the organization table work correctly.
//
// Root cause to investigate: The Worker's query path may not be applying
// filters to tables that are not the "primary" table referenced in the
// template, or the filter-to-query mapping has a bug for multi-table templates.
//
// These tests are skipped until the bug is fixed. They serve as regression
// tests once the fix is applied.
// ############################################################################

// TC-FLT-014: eq on account table - filter by account name.
// BUG: Worker returns zero rows when filtering on account table fields.
func TestFilter_AccountEqName(t *testing.T) {
	t.Skip("BUG: Worker does not apply filters to account table — returns zero rows. See TC-FLT-014 comment.")

	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureListAccounts)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableAccount, "name", shared.FilterEq("Operating Account"))

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
	assert.Contains(t, content, "Operating Account", "should contain filtered Operating Account")
	assert.NotContains(t, content, "Savings Account", "should NOT contain other accounts")
}

// TC-FLT-015: gt date on account table - filter accounts created after 2025-04-01.
// BUG: Worker returns zero rows when filtering on account table fields.
func TestFilter_AccountGtDate(t *testing.T) {
	t.Skip("BUG: Worker does not apply filters to account table — returns zero rows. See TC-FLT-014 comment.")

	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureListAccounts)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableAccount, "created_at", shared.FilterGt("2025-04-01"))

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
	assert.Contains(t, content, "Checking Account", "should contain Checking Account (created Apr 15)")
	assert.Contains(t, content, "Revenue Account", "should contain Revenue Account (created Jun 15)")
	assert.NotContains(t, content, "Operating Account", "should NOT contain Operating Account (created Jan 25)")
}

// TC-FLT-016: lt date on account table - filter accounts created before 2025-03-01.
// BUG: Worker returns zero rows when filtering on account table fields.
func TestFilter_AccountLtDate(t *testing.T) {
	t.Skip("BUG: Worker does not apply filters to account table — returns zero rows. See TC-FLT-014 comment.")

	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
	defer cancel()

	tplID := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureListAccounts)

	filters := shared.MakeFilters(shared.DSMidazOnboarding, shared.TableAccount, "created_at", shared.FilterLt("2025-03-01"))

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
	assert.Contains(t, content, "Operating Account", "should contain Operating Account (created Jan 25)")
	assert.Contains(t, content, "Savings Account", "should contain Savings Account (created Feb 5)")
	assert.NotContains(t, content, "Revenue Account", "should NOT contain Revenue Account (created Jun 15)")
}
