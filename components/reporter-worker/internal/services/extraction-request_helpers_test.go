// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
	extractionRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/extraction"
	reportData "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestUseCase_RequestFetcherExtraction_MultipleDatasources(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	callCount := 0

	mockFetcher := &mockExtractionJobCreator{
		createFunc: func(_ context.Context, jobReq fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
			callCount++
			assert.Len(t, jobReq.DataRequest.MappedFields, 2, "Expected 2 datasources in MappedFields")
			assert.Contains(t, jobReq.DataRequest.MappedFields, "onboarding")
			assert.Contains(t, jobReq.DataRequest.MappedFields, "plugin_crm")

			return &fetcher.ExtractionJobResponse{
				JobID:     "fetcher-job-multi",
				Status:    "accepted",
				CreatedAt: time.Now(),
			}, nil
		},
	}

	// Single extraction job = one mapping create
	mockExtractionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "PendingExtraction", reportID, gomock.Any(), nil).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(context.Background(), "test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		FetcherClient:         mockFetcher,
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
	}

	message := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "html",
		DataQueries: map[string]map[string][]string{
			"onboarding": {"organization": {"name"}},
			"plugin_crm": {"holders": {"document"}},
		},
	}

	err := uc.requestFetcherExtraction(context.Background(), message, &span)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "Expected one extraction job creation with all datasources in MappedFields")
}

func TestConvertToFetcherFilters(t *testing.T) {
	t.Parallel()

	t.Run("nil filters", func(t *testing.T) {
		t.Parallel()
		result := convertToFetcherFilters(nil)
		assert.Nil(t, result)
	})

	t.Run("empty filters", func(t *testing.T) {
		t.Parallel()
		result := convertToFetcherFilters(map[string]map[string]map[string]model.FilterCondition{})
		assert.Nil(t, result)
	})

	t.Run("converts equals condition", func(t *testing.T) {
		t.Parallel()
		filters := map[string]map[string]map[string]model.FilterCondition{
			"onboarding": {
				"organization": {
					"id": {Equals: []any{42}},
				},
			},
		}
		result := convertToFetcherFilters(filters)
		require.NotNil(t, result)
		assert.Equal(t, []any{42}, result["onboarding"]["organization"]["id"].Equals)
	})
}

func TestConvertMappedFieldsToDotNotation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    map[string]map[string][]string
		expected map[string]map[string][]string
	}{
		{
			name:     "nil input returns empty map",
			input:    nil,
			expected: map[string]map[string][]string{},
		},
		{
			name:     "empty input returns empty map",
			input:    map[string]map[string][]string{},
			expected: map[string]map[string][]string{},
		},
		{
			name: "converts double underscore to dot",
			input: map[string]map[string][]string{
				"onboarding": {
					"public__organization": {"id", "name"},
				},
			},
			expected: map[string]map[string][]string{
				"onboarding": {
					"public.organization": {"id", "name"},
				},
			},
		},
		{
			name: "preserves keys without double underscore",
			input: map[string]map[string][]string{
				"onboarding": {
					"organization": {"id", "name"},
				},
			},
			expected: map[string]map[string][]string{
				"onboarding": {
					"organization": {"id", "name"},
				},
			},
		},
		{
			name: "multiple datasources with mixed keys",
			input: map[string]map[string][]string{
				"onboarding": {
					"public__organization": {"id", "name"},
					"accounts":             {"balance"},
				},
				"plugin_crm": {
					"crm__holders": {"document", "email"},
				},
			},
			expected: map[string]map[string][]string{
				"onboarding": {
					"public.organization": {"id", "name"},
					"accounts":            {"balance"},
				},
				"plugin_crm": {
					"crm.holders": {"document", "email"},
				},
			},
		},
		{
			name: "multiple double underscores in same key",
			input: map[string]map[string][]string{
				"ds": {
					"catalog__schema__table": {"col"},
				},
			},
			expected: map[string]map[string][]string{
				"ds": {
					"catalog.schema.table": {"col"},
				},
			},
		},
		{
			name: "preserves field lists exactly",
			input: map[string]map[string][]string{
				"ds": {
					"t": {"a", "b", "c"},
				},
			},
			expected: map[string]map[string][]string{
				"ds": {
					"t": {"a", "b", "c"},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertMappedFieldsToDotNotation(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToFetcherFilters_DoubleUnderscoreConversion(t *testing.T) {
	t.Parallel()

	t.Run("converts table keys with double underscore to dot notation", func(t *testing.T) {
		t.Parallel()

		filters := map[string]map[string]map[string]model.FilterCondition{
			"onboarding": {
				"public__organization": {
					"status": {Equals: []any{"active"}},
				},
			},
		}

		result := convertToFetcherFilters(filters)
		require.NotNil(t, result)

		// The key should be converted from public__organization to public.organization
		require.Contains(t, result["onboarding"], "public.organization")
		assert.Equal(t, []any{"active"}, result["onboarding"]["public.organization"]["status"].Equals)
	})

	t.Run("converts all filter condition fields", func(t *testing.T) {
		t.Parallel()

		filters := map[string]map[string]map[string]model.FilterCondition{
			"ds": {
				"table": {
					"field": {
						Equals:         []any{"val"},
						GreaterThan:    []any{10},
						GreaterOrEqual: []any{5},
						LessThan:       []any{100},
						LessOrEqual:    []any{50},
						Between:        []any{1, 10},
						In:             []any{"a", "b"},
						NotIn:          []any{"c"},
					},
				},
			},
		}

		result := convertToFetcherFilters(filters)
		require.NotNil(t, result)

		fc := result["ds"]["table"]["field"]
		assert.Equal(t, []any{"val"}, fc.Equals)
		assert.Equal(t, []any{10}, fc.GreaterThan)
		assert.Equal(t, []any{5}, fc.GreaterOrEqual)
		assert.Equal(t, []any{100}, fc.LessThan)
		assert.Equal(t, []any{50}, fc.LessOrEqual)
		assert.Equal(t, []any{1, 10}, fc.Between)
		assert.Equal(t, []any{"a", "b"}, fc.In)
		assert.Equal(t, []any{"c"}, fc.NotIn)
	})

	t.Run("multiple datasources with multiple tables", func(t *testing.T) {
		t.Parallel()

		filters := map[string]map[string]map[string]model.FilterCondition{
			"onboarding": {
				"organization": {
					"id": {Equals: []any{1}},
				},
				"public__accounts": {
					"balance": {GreaterThan: []any{0}},
				},
			},
			"plugin_crm": {
				"crm__holders": {
					"status": {In: []any{"active", "pending"}},
				},
			},
		}

		result := convertToFetcherFilters(filters)
		require.NotNil(t, result)

		// onboarding datasource
		require.Contains(t, result, "onboarding")
		assert.Contains(t, result["onboarding"], "organization")
		assert.Contains(t, result["onboarding"], "public.accounts")
		assert.Equal(t, []any{1}, result["onboarding"]["organization"]["id"].Equals)
		assert.Equal(t, []any{0}, result["onboarding"]["public.accounts"]["balance"].GreaterThan)

		// plugin_crm datasource
		require.Contains(t, result, "plugin_crm")
		assert.Contains(t, result["plugin_crm"], "crm.holders")
		assert.Equal(t, []any{"active", "pending"}, result["plugin_crm"]["crm.holders"]["status"].In)
	})
}

func TestResolveTenantID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "nil context",
			ctx:      nil,
			expected: "",
		},
		{
			name:     "context without tenant ID",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "context with tenant ID",
			ctx:      tmcore.ContextWithTenantID(context.Background(), "tenant-123"),
			expected: "tenant-123",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := resolveTenantID(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}
