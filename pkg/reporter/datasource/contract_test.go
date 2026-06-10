// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Contract tests: the in-process DirectProvider MUST satisfy DataSourceProvider
// across both single-tenant and multi-tenant construction. These tests verify
// the mode-agnostic interface contract (error shapes, non-nil results) rather
// than implementation details. The remote Fetcher provider has been retired.
// ---------------------------------------------------------------------------

// Compile-time interface satisfaction check.
var _ DataSourceProvider = (*DirectProvider)(nil)

// --- Contract: Empty data source ID returns error ---------------------------

func TestContract_GetDataSourceSchema_EmptyID(t *testing.T) {
	provider := newContractDirectProvider(t)

	schema, err := provider.GetDataSourceSchema(context.Background(), "")

	require.Error(t, err, "must return error for empty dataSourceID")
	assert.Nil(t, schema, "must return nil schema for empty dataSourceID")
	assert.Contains(t, err.Error(), "data source ID must not be empty")
}

// --- Contract: Empty field list returns error -------------------------------

func TestContract_ValidateSchema_EmptyFields(t *testing.T) {
	provider := newContractDirectProvider(t)

	result, err := provider.ValidateSchema(context.Background(), "any-ds", map[string][]string{})

	require.Error(t, err, "must return error for empty tableFields")
	assert.Nil(t, result, "must return nil result for empty tableFields")
	assert.Contains(t, err.Error(), "tableFields must not be empty")
}

// --- Contract: ListDataSources returns slice (never nil on success) ---------

func TestContract_ListDataSources_ReturnsSlice(t *testing.T) {
	provider := newContractDirectProvider(t)

	result, err := provider.ListDataSources(context.Background())

	require.NoError(t, err, "must not error for empty datasource list")
	require.NotNil(t, result, "must return non-nil slice (may be empty)")
}

// --- Contract: HealthCheck returns map (never nil on success) ---------------

func TestContract_HealthCheck_ReturnsMap(t *testing.T) {
	provider := newContractDirectProvider(t)

	result, err := provider.HealthCheck(context.Background())

	require.NoError(t, err, "must not error for empty health check")
	require.NotNil(t, result, "must return non-nil map (may be empty)")
}

// --- Contract: GetDataSourceSchema unknown ID returns error wrapping sentinel

func TestContract_GetDataSourceSchema_UnknownID(t *testing.T) {
	dp := newContractDirectProvider(t)

	schema, err := dp.GetDataSourceSchema(context.Background(), "non-existent-id")

	require.Error(t, err, "DirectProvider must error for unknown datasource ID")
	assert.Nil(t, schema)
	assert.True(t, errors.Is(err, ErrDataSourceNotFound),
		"DirectProvider must wrap ErrDataSourceNotFound, got: %v", err)
}

// ---------------------------------------------------------------------------
// Contract test helpers — minimal wiring, no real DB/HTTP.
// ---------------------------------------------------------------------------

// newContractDirectProvider creates a DirectProvider with an empty SafeDataSources.
func newContractDirectProvider(t *testing.T) *DirectProvider {
	t.Helper()

	pkg.ResetRegisteredDataSourceIDsForTesting()

	return NewDirectProvider(pkg.NewSafeDataSources(nil), nil, nil)
}

// --- Contract: DataSourceInfo carries the registry's datasource identity ----

func TestContract_ListDataSources_CarriesIdentity(t *testing.T) {
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"ds-1"})

	dsMap := map[string]pkg.DataSource{
		"ds-1": {
			DatabaseType: "postgresql",
			Status:       libConstants.DataSourceStatusAvailable,
		},
	}

	dp := NewDirectProvider(pkg.NewSafeDataSources(dsMap), nil, nil)

	result, err := dp.ListDataSources(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "ds-1", result[0].ID)
	assert.Equal(t, "postgresql", result[0].Type)
}
