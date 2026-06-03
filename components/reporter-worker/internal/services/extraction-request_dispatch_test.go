// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/fetcher"
	extractionRepo "github.com/LerianStudio/reporter/pkg/mongodb/extraction"
	reportData "github.com/LerianStudio/reporter/pkg/mongodb/report"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestUseCase_GenerateReportData_DualModeDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupUC     func(ctrl *gomock.Controller, reportID, templateID uuid.UUID) *UseCase
		expectError bool
		errContains string
	}{
		{
			name: "fetcher mode - dispatches to fetcher",
			setupUC: func(ctrl *gomock.Controller, reportID, templateID uuid.UUID) *UseCase {
				mockReportDataRepo := reportData.NewMockRepository(ctrl)
				mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

				mockFetcher := &mockExtractionJobCreator{
					createFunc: func(_ context.Context, _ fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error) {
						return &fetcher.ExtractionJobResponse{
							JobID:     "fetcher-job-dual",
							Status:    "accepted",
							CreatedAt: time.Now(),
						}, nil
					},
				}

				mockExtractionRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)

				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "PendingExtraction", reportID, gomock.Any(), nil).
					Return(nil)

				return &UseCase{
					Logger:                log.NewNop(),
					Tracer:                noop.NewTracerProvider().Tracer("test"),
					FetcherClient:         mockFetcher,
					ExtractionMappingRepo: mockExtractionRepo,
					ReportDataRepo:        mockReportDataRepo,
				}
			},
			expectError: false,
		},
		{
			name: "direct mode - no fetcher client, empty data queries",
			setupUC: func(ctrl *gomock.Controller, reportID, templateID uuid.UUID) *UseCase {
				// pkg.NewSafeDataSources requires a non-nil map to avoid panics
				return &UseCase{
					Logger:              log.NewNop(),
					Tracer:              noop.NewTracerProvider().Tracer("test"),
					FetcherClient:       nil,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			// Direct mode with empty DataQueries succeeds (no-op on queryExternalData)
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			reportID := uuid.New()
			templateID := uuid.New()

			uc := tt.setupUC(ctrl, reportID, templateID)
			tracer := noop.NewTracerProvider().Tracer("test")
			_, span := tracer.Start(context.Background(), "test")

			dataQueries := map[string]map[string][]string{
				"onboarding": {"organization": {"name"}},
			}
			// In direct mode, use empty data queries to avoid datasource-not-found panic
			if uc.FetcherClient == nil {
				dataQueries = map[string]map[string][]string{}
			}

			message := GenerateReportMessage{
				TemplateID:   templateID,
				ReportID:     reportID,
				OutputFormat: "html",
				DataQueries:  dataQueries,
			}
			result := make(map[string]map[string][]map[string]any)

			err := uc.generateReportData(context.Background(), message, result, &span, reportID)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
