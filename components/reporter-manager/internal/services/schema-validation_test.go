// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_ValidateSchemaViaProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mappedFields map[string]map[string][]string
		mockSetup    func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider
		wantWarnings []datasource.ValidationWarning
		wantErr      bool
		errContains  string
	}{
		{
			name: "Success - All fields valid, no warnings",
			mappedFields: map[string]map[string][]string{
				"ds1": {
					"users": {"id", "name", "email"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds1", map[string][]string{
						"users": {"id", "name", "email"},
					}).
					Return(&datasource.ValidationResult{Valid: true}, nil)
				return mock
			},
			wantWarnings: nil,
			wantErr:      false,
		},
		{
			name: "Success - Datasource unavailable returns warning (D7)",
			mappedFields: map[string]map[string][]string{
				"ds_unavailable": {
					"orders": {"id", "amount"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds_unavailable", map[string][]string{
						"orders": {"id", "amount"},
					}).
					Return(&datasource.ValidationResult{
						Valid: true,
						Warnings: []datasource.ValidationWarning{
							{
								Field:   "ds_unavailable",
								Code:    datasource.WarningCodeDataSourceUnavailable,
								Message: "Data source \"ds_unavailable\" is currently unavailable; validation skipped",
							},
						},
					}, nil)
				return mock
			},
			wantWarnings: []datasource.ValidationWarning{
				{
					Field:   "ds_unavailable",
					Code:    datasource.WarningCodeDataSourceUnavailable,
					Message: "Data source \"ds_unavailable\" is currently unavailable; validation skipped",
				},
			},
			wantErr: false,
		},
		{
			name: "Error - Schema validation finds invalid fields",
			mappedFields: map[string]map[string][]string{
				"ds1": {
					"users": {"id", "nonexistent_field"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds1", map[string][]string{
						"users": {"id", "nonexistent_field"},
					}).
					Return(&datasource.ValidationResult{Valid: false}, nil)
				return mock
			},
			wantWarnings: nil,
			wantErr:      true,
			errContains:  "TPL-0059",
		},
		{
			name: "Error - Provider returns error",
			mappedFields: map[string]map[string][]string{
				"ds_error": {
					"table1": {"col1"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds_error", map[string][]string{
						"table1": {"col1"},
					}).
					Return(nil, errors.New("connection refused"))
				return mock
			},
			wantWarnings: nil,
			wantErr:      true,
			errContains:  "connection refused",
		},
		{
			name: "Success - Multiple datasources, one with warning",
			mappedFields: map[string]map[string][]string{
				"ds_ok": {
					"accounts": {"id", "balance"},
				},
				"ds_down": {
					"transactions": {"id", "amount"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds_ok", map[string][]string{
						"accounts": {"id", "balance"},
					}).
					Return(&datasource.ValidationResult{Valid: true}, nil)
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds_down", map[string][]string{
						"transactions": {"id", "amount"},
					}).
					Return(&datasource.ValidationResult{
						Valid: true,
						Warnings: []datasource.ValidationWarning{
							{
								Field:   "ds_down",
								Code:    datasource.WarningCodeDataSourceUnavailable,
								Message: "Data source \"ds_down\" is currently unavailable; validation skipped",
							},
						},
					}, nil)
				return mock
			},
			wantWarnings: []datasource.ValidationWarning{
				{
					Field:   "ds_down",
					Code:    datasource.WarningCodeDataSourceUnavailable,
					Message: "Data source \"ds_down\" is currently unavailable; validation skipped",
				},
			},
			wantErr: false,
		},
		{
			name: "Success - Multiple tables in same datasource, fields collected",
			mappedFields: map[string]map[string][]string{
				"ds1": {
					"users":    {"id", "name"},
					"accounts": {"id", "balance"},
				},
			},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				mock := datasource.NewMockDataSourceProvider(ctrl)
				// Table→fields map is passed directly to the provider
				mock.EXPECT().
					ValidateSchema(gomock.Any(), "ds1", gomock.Any()).
					Return(&datasource.ValidationResult{Valid: true}, nil)
				return mock
			},
			wantWarnings: nil,
			wantErr:      false,
		},
		{
			name:         "Success - Empty mapped fields, no validation needed",
			mappedFields: map[string]map[string][]string{},
			mockSetup: func(ctrl *gomock.Controller) *datasource.MockDataSourceProvider {
				return datasource.NewMockDataSourceProvider(ctrl)
			},
			wantWarnings: nil,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProvider := tt.mockSetup(ctrl)

			svc := &UseCase{
				Logger:             log.NewNop(),
				Tracer:             noop.NewTracerProvider().Tracer("test"),
				DataSourceProvider: mockProvider,
			}

			ctx := context.Background()
			warnings, err := svc.ValidateSchemaViaProvider(ctx, tt.mappedFields)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}

			if tt.wantWarnings != nil {
				require.NotNil(t, warnings)
				assert.Len(t, warnings, len(tt.wantWarnings))
				for i, w := range tt.wantWarnings {
					assert.Equal(t, w.Code, warnings[i].Code)
					assert.Equal(t, w.Field, warnings[i].Field)
				}
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestUseCase_ValidateSchemaViaProvider_NilProvider(t *testing.T) {
	t.Parallel()

	svc := &UseCase{
		Logger:             log.NewNop(),
		Tracer:             noop.NewTracerProvider().Tracer("test"),
		DataSourceProvider: nil,
	}

	ctx := context.Background()
	warnings, err := svc.ValidateSchemaViaProvider(ctx, map[string]map[string][]string{
		"ds1": {"table": {"col1"}},
	})

	require.NoError(t, err, "nil provider should skip validation without error")
	assert.Empty(t, warnings, "nil provider should produce no warnings")
}
