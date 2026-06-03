// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/fetcher"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check: FetcherProvider must implement DataSourceProvider.
var _ DataSourceProvider = (*FetcherProvider)(nil)

// mockFetcherClient implements FetcherManagementClient for testing.
type mockFetcherClient struct {
	listConnectionsFn     func(ctx context.Context) ([]fetcher.ConnectionResponse, error)
	getConnectionSchemaFn func(ctx context.Context, connectionID string) (*fetcher.ConnectionSchemaResponse, error)
	validateSchemaFn      func(ctx context.Context, mappedFields map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error)
	pingFn                func(ctx context.Context) error
}

func (m *mockFetcherClient) ListConnections(ctx context.Context) ([]fetcher.ConnectionResponse, error) {
	if m.listConnectionsFn == nil {
		return nil, nil
	}

	return m.listConnectionsFn(ctx)
}

func (m *mockFetcherClient) GetConnectionSchema(ctx context.Context, connectionID string) (*fetcher.ConnectionSchemaResponse, error) {
	return m.getConnectionSchemaFn(ctx, connectionID)
}

func (m *mockFetcherClient) ValidateSchema(ctx context.Context, mappedFields map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error) {
	return m.validateSchemaFn(ctx, mappedFields)
}

func (m *mockFetcherClient) Ping(ctx context.Context) error {
	if m.pingFn == nil {
		return nil
	}

	return m.pingFn(ctx)
}

func TestFetcherProvider_Constructor(t *testing.T) {
	mock := &mockFetcherClient{}
	provider := NewFetcherProvider(mock)

	require.NotNil(t, provider)
}

func TestFetcherProvider_ListDataSources(t *testing.T) {
	tests := []struct {
		name        string
		mockFn      func(ctx context.Context) ([]fetcher.ConnectionResponse, error)
		wantLen     int
		wantErr     bool
		wantErrMsg  string
		checkResult func(t *testing.T, result []DataSourceInfo)
	}{
		{
			name: "maps connections to datasource info",
			mockFn: func(_ context.Context) ([]fetcher.ConnectionResponse, error) {
				return []fetcher.ConnectionResponse{
					{ID: "pg-uuid-123", ConfigName: "midaz_onboarding", Type: "postgresql"},
					{ID: "mongo-uuid-456", ConfigName: "plugin_crm", Type: "mongodb"},
				}, nil
			},
			wantLen: 2,
			wantErr: false,
			checkResult: func(t *testing.T, result []DataSourceInfo) {
				t.Helper()

				// Build map for order-independent assertion (ID is UUID)
				byID := make(map[string]DataSourceInfo, len(result))
				for _, r := range result {
					byID[r.ID] = r
				}

				pg := byID["pg-uuid-123"]
				assert.Equal(t, "midaz_onboarding", pg.Name)
				assert.Equal(t, "postgresql", pg.Type)

				mongo := byID["mongo-uuid-456"]
				assert.Equal(t, "plugin_crm", mongo.Name)
				assert.Equal(t, "mongodb", mongo.Type)
			},
		},
		{
			name: "returns empty list when no connections",
			mockFn: func(_ context.Context) ([]fetcher.ConnectionResponse, error) {
				return []fetcher.ConnectionResponse{}, nil
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "propagates client error",
			mockFn: func(_ context.Context) ([]fetcher.ConnectionResponse, error) {
				return nil, errors.New("connection refused")
			},
			wantErr:    true,
			wantErrMsg: "failed to list data sources from fetcher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockFetcherClient{
				listConnectionsFn: tt.mockFn,
			}
			provider := NewFetcherProvider(mock)

			result, err := provider.ListDataSources(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestFetcherProvider_GetDataSourceSchema(t *testing.T) {
	tests := []struct {
		name       string
		dsID       string
		mockFn     func(ctx context.Context, id string) (*fetcher.ConnectionSchemaResponse, error)
		wantErr    bool
		wantErrMsg string
		check      func(t *testing.T, schema *DataSourceSchema)
	}{
		{
			name: "maps fetcher schema to internal schema",
			dsID: "pg-main",
			mockFn: func(_ context.Context, _ string) (*fetcher.ConnectionSchemaResponse, error) {
				return &fetcher.ConnectionSchemaResponse{
					ID: "pg-main",
					Tables: []fetcher.SchemaTableResponse{
						{
							Name: "users",
							Fields: []fetcher.SchemaFieldResponse{
								{Name: "id", Type: "uuid"},
								{Name: "name", Type: "varchar"},
							},
						},
					},
				}, nil
			},
			wantErr: false,
			check: func(t *testing.T, schema *DataSourceSchema) {
				t.Helper()
				assert.Equal(t, "pg-main", schema.DataSourceID)
				require.Len(t, schema.Tables, 1)
				assert.Equal(t, "users", schema.Tables[0].Name)
				require.Len(t, schema.Tables[0].Fields, 2)
				assert.Equal(t, "id", schema.Tables[0].Fields[0].Name)
				assert.Equal(t, "uuid", schema.Tables[0].Fields[0].Type)
			},
		},
		{
			name:       "rejects empty data source ID",
			dsID:       "",
			wantErr:    true,
			wantErrMsg: "data source ID must not be empty",
		},
		{
			name: "propagates client error",
			dsID: "pg-main",
			mockFn: func(_ context.Context, _ string) (*fetcher.ConnectionSchemaResponse, error) {
				return nil, errors.New("not found")
			},
			wantErr:    true,
			wantErrMsg: "failed to get schema from fetcher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockFetcherClient{
				getConnectionSchemaFn: tt.mockFn,
			}
			provider := NewFetcherProvider(mock)

			schema, err := provider.GetDataSourceSchema(context.Background(), tt.dsID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, schema)

			if tt.check != nil {
				tt.check(t, schema)
			}
		})
	}
}
