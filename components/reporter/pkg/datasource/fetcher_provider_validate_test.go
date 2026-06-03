// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetcherProvider_ValidateSchema(t *testing.T) {
	tests := []struct {
		name        string
		dsID        string
		tableFields map[string][]string
		mockFn      func(ctx context.Context, mappedFields map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error)
		wantErr     bool
		wantErrMsg  string
		check       func(t *testing.T, result *ValidationResult)
	}{
		{
			name:        "returns valid result when all fields exist",
			dsID:        "pg-main",
			tableFields: map[string][]string{"users": {"id", "name"}},
			mockFn: func(_ context.Context, _ map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error) {
				return &fetcher.ValidateSchemaResponse{
					Status:  "success",
					Message: "All tables and fields validated successfully.",
				}, nil
			},
			wantErr: false,
			check: func(t *testing.T, result *ValidationResult) {
				t.Helper()
				assert.True(t, result.Valid)
				assert.Empty(t, result.Warnings)
			},
		},
		{
			name:        "maps DATA_SOURCE_DOWN to warning per D7",
			dsID:        "pg-down",
			tableFields: map[string][]string{"users": {"id"}},
			mockFn: func(_ context.Context, _ map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error) {
				return &fetcher.ValidateSchemaResponse{
					Status:  "failure",
					Message: "Schema validation failed.",
					Errors: []fetcher.SchemaValidationError{
						{
							Type:         "DATA_SOURCE_DOWN",
							DataSourceID: "pg-down",
						},
					},
				}, nil
			},
			wantErr: false,
			check: func(t *testing.T, result *ValidationResult) {
				t.Helper()
				assert.False(t, result.Valid)
				require.Len(t, result.Warnings, 1)
				assert.Equal(t, WarningCodeDataSourceUnavailable, result.Warnings[0].Code)
				assert.Equal(t, "pg-down", result.Warnings[0].Field)
			},
		},
		{
			name:        "rejects empty tableFields",
			dsID:        "pg-main",
			tableFields: map[string][]string{},
			wantErr:     true,
			wantErrMsg:  "tableFields must not be empty",
		},
		{
			name:        "rejects nil tableFields",
			dsID:        "pg-main",
			tableFields: nil,
			wantErr:     true,
			wantErrMsg:  "tableFields must not be empty",
		},
		{
			name:        "propagates client error",
			dsID:        "pg-main",
			tableFields: map[string][]string{"users": {"id"}},
			mockFn: func(_ context.Context, _ map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error) {
				return nil, errors.New("validate failed")
			},
			wantErr:    true,
			wantErrMsg: "failed to validate schema via fetcher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockFetcherClient{
				validateSchemaFn: tt.mockFn,
			}
			provider := NewFetcherProvider(mock)

			result, err := provider.ValidateSchema(context.Background(), tt.dsID, tt.tableFields)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestFetcherProvider_HealthCheck(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(ctx context.Context) ([]fetcher.ConnectionResponse, error)
		wantErr    bool
		wantErrMsg string
		check      func(t *testing.T, result map[string]bool)
	}{
		{
			name: "reports healthy connections as true",
			mockFn: func(_ context.Context) ([]fetcher.ConnectionResponse, error) {
				return []fetcher.ConnectionResponse{
					{ID: "pg-main"},
				}, nil
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]bool) {
				t.Helper()
				assert.True(t, result["pg-main"])
			},
		},
		{
			name: "returns empty map when no connections",
			mockFn: func(_ context.Context) ([]fetcher.ConnectionResponse, error) {
				return []fetcher.ConnectionResponse{}, nil
			},
			wantErr: false,
			check: func(t *testing.T, result map[string]bool) {
				t.Helper()
				assert.Empty(t, result)
			},
		},
		{
			name: "returns error when client fails",
			mockFn: func(_ context.Context) ([]fetcher.ConnectionResponse, error) {
				return nil, errors.New("connection refused")
			},
			wantErr:    true,
			wantErrMsg: "failed to check fetcher health",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockFetcherClient{
				listConnectionsFn: tt.mockFn,
			}
			provider := NewFetcherProvider(mock)

			result, err := provider.HealthCheck(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestConvertSchemaNotation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "converts dot notation to double underscore",
			input: "public.users",
			want:  "public__users",
		},
		{
			name:  "leaves plain name unchanged",
			input: "users",
			want:  "users",
		},
		{
			name:  "handles multiple dots",
			input: "catalog.public.users",
			want:  "catalog__public__users",
		},
		{
			name:  "handles empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertSchemaNotation(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMapWarningCode(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "maps DATA_SOURCE_DOWN to DATA_SOURCE_UNAVAILABLE",
			code: "DATA_SOURCE_DOWN",
			want: WarningCodeDataSourceUnavailable,
		},
		{
			name: "passes through unknown codes unchanged",
			code: "SOME_OTHER_WARNING",
			want: "SOME_OTHER_WARNING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapWarningCode(tt.code)
			assert.Equal(t, tt.want, got)
		})
	}
}
