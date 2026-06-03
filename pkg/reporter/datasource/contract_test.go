// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/fetcher"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Contract tests: Both DirectProvider and FetcherProvider MUST satisfy
// DataSourceProvider. These tests verify behavioral equivalence (same inputs
// produce structurally equivalent outputs) rather than implementation details.
// ---------------------------------------------------------------------------

// Compile-time interface satisfaction checks.
var (
	_ DataSourceProvider      = (*DirectProvider)(nil)
	_ DataSourceProvider      = (*FetcherProvider)(nil)
	_ FetcherManagementClient = (*fetcher.FetcherClient)(nil)
)

// --- Contract: Empty data source ID returns error for both providers --------

func TestContract_GetDataSourceSchema_EmptyID(t *testing.T) {
	providers := map[string]DataSourceProvider{
		"DirectProvider":  newContractDirectProvider(t),
		"FetcherProvider": newContractFetcherProvider(t),
	}

	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			schema, err := provider.GetDataSourceSchema(context.Background(), "")

			require.Error(t, err, "%s must return error for empty dataSourceID", name)
			assert.Nil(t, schema, "%s must return nil schema for empty dataSourceID", name)
			assert.Contains(t, err.Error(), "data source ID must not be empty",
				"%s must use consistent error message for empty ID", name)
		})
	}
}

// --- Contract: Empty field list returns error for both providers ------------

func TestContract_ValidateSchema_EmptyFields(t *testing.T) {
	providers := map[string]DataSourceProvider{
		"DirectProvider":  newContractDirectProvider(t),
		"FetcherProvider": newContractFetcherProvider(t),
	}

	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			result, err := provider.ValidateSchema(context.Background(), "any-ds", map[string][]string{})

			require.Error(t, err, "%s must return error for empty tableFields", name)
			assert.Nil(t, result, "%s must return nil result for empty tableFields", name)
			assert.Contains(t, err.Error(), "tableFields must not be empty",
				"%s must use consistent error message for empty tableFields", name)
		})
	}
}

// --- Contract: ListDataSources returns slice (never nil on success) ---------

func TestContract_ListDataSources_ReturnsSlice(t *testing.T) {
	providers := map[string]DataSourceProvider{
		"DirectProvider":  newContractDirectProvider(t),
		"FetcherProvider": newContractFetcherProviderWithConnections(t, nil),
	}

	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			result, err := provider.ListDataSources(context.Background())

			require.NoError(t, err, "%s must not error for empty datasource list", name)
			require.NotNil(t, result, "%s must return non-nil slice (may be empty)", name)
		})
	}
}

// --- Contract: HealthCheck returns map (never nil on success) ---------------

func TestContract_HealthCheck_ReturnsMap(t *testing.T) {
	providers := map[string]DataSourceProvider{
		"DirectProvider":  newContractDirectProvider(t),
		"FetcherProvider": newContractFetcherProviderWithConnections(t, nil),
	}

	for name, provider := range providers {
		t.Run(name, func(t *testing.T) {
			result, err := provider.HealthCheck(context.Background())

			require.NoError(t, err, "%s must not error for empty health check", name)
			require.NotNil(t, result, "%s must return non-nil map (may be empty)", name)
		})
	}
}

// --- Contract: GetDataSourceSchema unknown ID returns error wrapping sentinel

func TestContract_GetDataSourceSchema_UnknownID(t *testing.T) {
	// DirectProvider wraps ErrDataSourceNotFound for unknown IDs.
	dp := newContractDirectProvider(t)
	schema, err := dp.GetDataSourceSchema(context.Background(), "non-existent-id")

	require.Error(t, err, "DirectProvider must error for unknown datasource ID")
	assert.Nil(t, schema)
	assert.True(t, errors.Is(err, ErrDataSourceNotFound),
		"DirectProvider must wrap ErrDataSourceNotFound, got: %v", err)
}

// --- Config Validation Contract Tests ---------------------------------------

func TestContract_ValidateProviderConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        ProviderConfig
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "fails when FetcherEnabled but FetcherURL empty",
			cfg: ProviderConfig{
				FetcherEnabled: true,
				FetcherURL:     "",
			},
			wantErr:    true,
			wantErrMsg: "FETCHER_ENABLED=true requires FETCHER_URL",
		},
		{
			name: "fails when MultiTenantEnabled but FetcherEnabled false",
			cfg: ProviderConfig{
				FetcherEnabled:     false,
				MultiTenantEnabled: true,
			},
			wantErr:    true,
			wantErrMsg: "MULTI_TENANT_ENABLED=true requires FETCHER_ENABLED=true",
		},
		{
			name: "valid direct mode config",
			cfg: ProviderConfig{
				FetcherEnabled:     false,
				MultiTenantEnabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid fetcher mode config",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "http://fetcher:4007",
				MultiTenantEnabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid fetcher + multi-tenant config",
			cfg: ProviderConfig{
				FetcherEnabled:     true,
				FetcherURL:         "http://fetcher:4007",
				MultiTenantEnabled: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
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

// contractFetcherClient is a minimal stub satisfying FetcherManagementClient
// for contract tests. It does NOT test Fetcher HTTP behavior — only that
// FetcherProvider handles the interface contract correctly.
type contractFetcherClient struct {
	connections []fetcher.ConnectionResponse
	schemaResp  *fetcher.ConnectionSchemaResponse
	schemaErr   error
	validateErr error
}

func (c *contractFetcherClient) ListConnections(_ context.Context) ([]fetcher.ConnectionResponse, error) {
	if c.connections == nil {
		return []fetcher.ConnectionResponse{}, nil
	}

	return c.connections, nil
}

func (c *contractFetcherClient) GetConnectionSchema(_ context.Context, _ string) (*fetcher.ConnectionSchemaResponse, error) {
	if c.schemaErr != nil {
		return nil, c.schemaErr
	}

	if c.schemaResp != nil {
		return c.schemaResp, nil
	}

	return &fetcher.ConnectionSchemaResponse{}, nil
}

func (c *contractFetcherClient) ValidateSchema(_ context.Context, _ map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error) {
	if c.validateErr != nil {
		return nil, c.validateErr
	}

	return &fetcher.ValidateSchemaResponse{Status: "success", Message: "All tables and fields validated successfully."}, nil
}

func (c *contractFetcherClient) Ping(_ context.Context) error {
	return nil
}

// newContractFetcherProvider creates a FetcherProvider with a stub client.
func newContractFetcherProvider(t *testing.T) *FetcherProvider {
	t.Helper()

	return NewFetcherProvider(&contractFetcherClient{})
}

// newContractFetcherProviderWithConnections creates a FetcherProvider with
// specific connections for list/health tests.
func newContractFetcherProviderWithConnections(t *testing.T, conns []fetcher.ConnectionResponse) *FetcherProvider {
	t.Helper()

	return NewFetcherProvider(&contractFetcherClient{connections: conns})
}

// --- Contract: DataSourceInfo structural equivalence -----------------------

func TestContract_ListDataSources_StructuralEquivalence(t *testing.T) {
	// DirectProvider with one datasource.
	pkg.ResetRegisteredDataSourceIDsForTesting()
	pkg.RegisterDataSourceIDsForTesting([]string{"ds-1"})

	dsMap := map[string]pkg.DataSource{
		"ds-1": {
			DatabaseType: "postgresql",
			Status:       libConstants.DataSourceStatusAvailable,
		},
	}

	dp := NewDirectProvider(pkg.NewSafeDataSources(dsMap), nil, nil)

	directResult, err := dp.ListDataSources(context.Background())
	require.NoError(t, err)
	require.Len(t, directResult, 1)

	// FetcherProvider with equivalent connection.
	fp := NewFetcherProvider(&contractFetcherClient{
		connections: []fetcher.ConnectionResponse{
			{
				ID:         "ds-1",
				ConfigName: "onboarding",
				Type:       "postgresql",
			},
		},
	})

	fetcherResult, err := fp.ListDataSources(context.Background())
	require.NoError(t, err)
	require.Len(t, fetcherResult, 1)

	// Both produce DataSourceInfo with the same structural fields.
	assert.Equal(t, directResult[0].ID, fetcherResult[0].ID,
		"Both providers must return same datasource ID")
	assert.Equal(t, directResult[0].Type, fetcherResult[0].Type,
		"Both providers must return same datasource Type")
}
