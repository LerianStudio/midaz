// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	shared "github.com/LerianStudio/midaz/v3/components/reporter/tests/e2e/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-DS-001: GET /v1/data-sources returns list containing all configured datasources.
func TestDS_ListDataSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, dataSources, err := apiClient.GetDataSources(ctx)
	require.NoError(t, err, "GetDataSources should not return an error")
	assert.Equal(t, http.StatusOK, status, "should return 200")
	require.GreaterOrEqual(t, len(dataSources), 3, "should have at least 3 data sources")

	foundOnboarding := false
	foundTransaction := false
	foundCRM := false

	for _, ds := range dataSources {
		switch ds.ID {
		case shared.DSMidazOnboarding:
			foundOnboarding = true

			assert.Equal(t, "postgresql", ds.Type, "midaz_onboarding should be postgresql")
		case shared.DSMidazTransaction:
			foundTransaction = true

			assert.Equal(t, "postgresql", ds.Type, "midaz_transaction should be postgresql")
		case shared.DSPluginCRM:
			foundCRM = true

			assert.Equal(t, "mongodb", ds.Type, "plugin_crm should be mongodb")
		}
	}

	assert.True(t, foundOnboarding, "data sources should contain %s", shared.DSMidazOnboarding)
	assert.True(t, foundTransaction, "data sources should contain %s", shared.DSMidazTransaction)
	assert.True(t, foundCRM, "data sources should contain %s", shared.DSPluginCRM)
}

// TC-DS-002: GET /v1/data-sources/midaz_onboarding returns tables with fields.
func TestDS_GetMidazOnboarding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, details, err := apiClient.GetDataSourceByID(ctx, shared.DSMidazOnboarding)
	require.NoError(t, err, "GetDataSourceByID should not return an error")
	assert.Equal(t, http.StatusOK, status, "should return 200")
	assert.Equal(t, shared.DSMidazOnboarding, details.ID, "ID should match")
	assert.Equal(t, "postgresql", details.Type, "type should be postgresql")
	require.NotEmpty(t, details.Tables, "tables should not be empty")

	// The API returns table names in "schema.table" format via QualifiedName()
	// (e.g., "public.organization" instead of just "organization").
	expectedTables := []string{
		shared.QualifiedTableOrganization,
		shared.QualifiedTableLedger,
		shared.QualifiedTableAccount,
	}

	tableNames := make(map[string]bool)
	for _, tbl := range details.Tables {
		tableNames[tbl.Name] = true

		assert.NotEmpty(t, tbl.Fields, "table %q should have fields", tbl.Name)
	}

	for _, expected := range expectedTables {
		assert.True(t, tableNames[expected], "tables should contain %q", expected)
	}
}

// TC-DS-003: GET /v1/data-sources/plugin_crm returns tables with fields.
func TestDS_GetPluginCRM(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, details, err := apiClient.GetDataSourceByID(ctx, shared.DSPluginCRM)
	require.NoError(t, err, "GetDataSourceByID should not return an error")

	if status == http.StatusInternalServerError {
		t.Skip("plugin_crm MongoDB connection unavailable (500) — skipping")
	}

	assert.Equal(t, http.StatusOK, status, "should return 200")
	assert.Equal(t, shared.DSPluginCRM, details.ID, "ID should match")
	assert.Equal(t, "mongodb", details.Type, "type should be mongodb")
	require.NotEmpty(t, details.Tables, "tables should not be empty")

	foundHolders := false

	for _, tbl := range details.Tables {
		if tbl.Name == shared.CollectionHolders {
			foundHolders = true

			assert.NotEmpty(t, tbl.Fields, "holders collection should have fields")
		}
	}

	assert.True(t, foundHolders, "tables should contain %q collection", shared.CollectionHolders)
}

// TC-DS-003b: GET /v1/data-sources/midaz_transaction returns tables with fields.
// This datasource shares the same PostgreSQL instance as midaz_onboarding
// but exposes transaction-related tables (operation, operation_route).
func TestDS_GetMidazTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	status, details, err := apiClient.GetDataSourceByID(ctx, shared.DSMidazTransaction)
	require.NoError(t, err, "GetDataSourceByID should not return an error")
	assert.Equal(t, http.StatusOK, status, "should return 200")
	assert.Equal(t, shared.DSMidazTransaction, details.ID, "ID should match")
	assert.Equal(t, "postgresql", details.Type, "type should be postgresql")
	require.NotEmpty(t, details.Tables, "tables should not be empty")

	expectedTables := []string{
		shared.QualifiedTableOperationRoute,
		shared.QualifiedTableOperation,
	}

	tableNames := make(map[string]bool)
	for _, tbl := range details.Tables {
		tableNames[tbl.Name] = true

		assert.NotEmpty(t, tbl.Fields, "table %q should have fields", tbl.Name)
	}

	for _, expected := range expectedTables {
		assert.True(t, tableNames[expected], "tables should contain %q", expected)
	}
}

// TC-DS-004: GET /v1/data-sources/nonexistent_ds returns 400.
// The API rejects unknown data source IDs via IsValidDataSourceID() with ErrMissingDataSource,
// which maps to a ValidationError (400), not EntityNotFoundError (404).
func TestDS_GetNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetDataSourceByIDRaw(ctx, "nonexistent_ds")
	require.NoError(t, err, "GetDataSourceByID should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for unknown data source ID (rejected by IsValidDataSourceID)")

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response body should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0031")
}

// TC-DS-005: GET /v1/data-sources/../../etc/passwd returns 400 (path traversal).
func TestDS_GetPathTraversal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	testCases := []struct {
		name  string
		input string
	}{
		{name: "path traversal", input: "../../etc/passwd"},
		{name: "digit start", input: "0_starts_with_digit"},
		{name: "special chars", input: "a!@#$"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp, err := apiClient.GetDataSourceByIDRaw(ctx, tc.input)
			require.NoError(t, err, "request should not return a transport error for input %q", tc.input)
			// Path traversal and invalid IDs may return 400 (validation error) or 404 (route not matched).
			assert.True(t, resp.StatusCode() == http.StatusBadRequest || resp.StatusCode() == http.StatusNotFound,
				"should return 400 or 404 for invalid input %q, got %d", tc.input, resp.StatusCode())
		})
	}
}

// TC-DS-006: Schema caching - second call should succeed (optionally faster).
func TestDS_SchemaCaching(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// First call (cache miss).
	start1 := time.Now()

	status1, details1, err := apiClient.GetDataSourceByID(ctx, shared.DSMidazOnboarding)
	elapsed1 := time.Since(start1)

	require.NoError(t, err, "first call should not return an error")
	assert.Equal(t, http.StatusOK, status1, "first call should return 200")
	require.NotEmpty(t, details1.Tables, "first call should return tables")

	// Second call (cache hit).
	start2 := time.Now()

	status2, details2, err := apiClient.GetDataSourceByID(ctx, shared.DSMidazOnboarding)
	elapsed2 := time.Since(start2)

	require.NoError(t, err, "second call should not return an error")
	assert.Equal(t, http.StatusOK, status2, "second call should return 200")
	require.NotEmpty(t, details2.Tables, "second call should return tables")

	// Verify both calls return identical data.
	assert.Equal(t, len(details1.Tables), len(details2.Tables), "both calls should return the same number of tables")

	// Soft assertion: log timing for observability. Do not fail on timing.
	t.Logf("first call: %v, second call: %v (expected second to be faster due to caching)", elapsed1, elapsed2)
}
