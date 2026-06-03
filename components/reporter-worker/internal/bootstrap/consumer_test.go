// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter-worker/internal/services"
	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	reportData "github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/seaweedfs/template"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestMultiQueueConsumer_HandlerGenerateReport_ErrorClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		mockSetup          func(ctrl *gomock.Controller) *services.UseCase
		expectedSpanStatus codes.Code
	}{
		{
			name: "Error - Business error (ValidationError) should keep span status OK",
			mockSetup: func(ctrl *gomock.Controller) *services.UseCase {
				mockTemplateRepo := template.NewMockRepository(ctrl)
				mockReportDataRepo := reportData.NewMockRepository(ctrl)

				reportID := uuid.New()

				// FindByID returns a report in processing state so it doesn't skip
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "processing",
					}, nil)

				// Template repo returns a business error (ValidationError)
				mockTemplateRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidationError{
						Code:    "TPL-0001",
						Title:   "Missing required fields",
						Message: "template validation failed",
					})

				// UpdateReportStatusById is called by handleErrorWithUpdate -> updateReportWithErrors
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				return &services.UseCase{
					Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
					TemplateSeaweedFS:   mockTemplateRepo,
					ReportDataRepo:      mockReportDataRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			// Business errors should NOT set span to ERROR -- span stays Unset (OK)
			expectedSpanStatus: codes.Unset,
		},
		{
			name: "Error - Technical error (infra failure) should set span status to ERROR",
			mockSetup: func(ctrl *gomock.Controller) *services.UseCase {
				mockTemplateRepo := template.NewMockRepository(ctrl)
				mockReportDataRepo := reportData.NewMockRepository(ctrl)

				reportID := uuid.New()

				// FindByID returns a report in processing state so it doesn't skip
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "processing",
					}, nil)

				// Template repo returns a technical error (infra failure)
				mockTemplateRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection refused: storage service unavailable"))

				// UpdateReportStatusById is called by handleErrorWithUpdate -> updateReportWithErrors
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				return &services.UseCase{
					Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
					TemplateSeaweedFS:   mockTemplateRepo,
					ReportDataRepo:      mockReportDataRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			// Technical errors MUST set span to ERROR
			expectedSpanStatus: codes.Error,
		},
		{
			name: "Error - Business error (EntityNotFoundError) should keep span status OK",
			mockSetup: func(ctrl *gomock.Controller) *services.UseCase {
				mockTemplateRepo := template.NewMockRepository(ctrl)
				mockReportDataRepo := reportData.NewMockRepository(ctrl)

				reportID := uuid.New()

				// FindByID returns a report in processing state
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "processing",
					}, nil)

				// Template repo returns an EntityNotFoundError (business error)
				mockTemplateRepo.EXPECT().
					Get(gomock.Any(), gomock.Any()).
					Return(nil, pkg.EntityNotFoundError{
						Code:    "TPL-0010",
						Title:   "Entity Not Found",
						Message: "template not found in storage",
					})

				// UpdateReportStatusById is called by the error handling path
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)

				return &services.UseCase{
					Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
					TemplateSeaweedFS:   mockTemplateRepo,
					ReportDataRepo:      mockReportDataRepo,
					ExternalDataSources: pkg.NewSafeDataSources(map[string]pkg.DataSource{}),
				}
			},
			// Business errors should NOT set span to ERROR -- span stays Unset (OK)
			expectedSpanStatus: codes.Unset,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			useCase := tt.mockSetup(ctrl)

			// Set up an in-memory span exporter to capture spans
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer func() { _ = tp.Shutdown(context.Background()) }()

			tracer := tp.Tracer("test")

			// Override UseCase's Tracer with the SDK tracer so spans are recorded
			useCase.Logger = &log.NopLogger{}
			useCase.Tracer = tracer

			// Build context with lib-commons tracking components
			ctx := libObservability.ContextWithTracer(
				libObservability.ContextWithLogger(
					libObservability.ContextWithHeaderID(context.Background(), "test-request-id"),
					&log.NopLogger{},
				),
				tracer,
			)

			mq := &MultiQueueConsumer{
				UseCase: useCase,
				logger:  &log.NopLogger{},
			}

			templateID := uuid.New()
			reportID := uuid.New()

			body := services.GenerateReportMessage{
				TemplateID:   templateID,
				ReportID:     reportID,
				OutputFormat: "txt",
				DataQueries:  map[string]map[string][]string{},
			}
			bodyBytes, err := json.Marshal(body)
			require.NoError(t, err)

			// Call the handler -- it should return an error
			handlerErr := mq.handlerGenerateReport(ctx, bodyBytes)
			require.Error(t, handlerErr)

			// Force flush to ensure spans are exported
			err = tp.ForceFlush(context.Background())
			require.NoError(t, err)

			// Find the handler span (handler.report.generate)
			spans := exporter.GetSpans()
			require.NotEmpty(t, spans, "expected at least one span to be recorded")

			var handlerSpan tracetest.SpanStub
			found := false

			for _, s := range spans {
				if s.Name == "handler.report.generate" {
					handlerSpan = s
					found = true

					break
				}
			}

			require.True(t, found, "expected to find span named 'handler.report.generate', got spans: %v", spanNames(spans))

			assert.Equal(t, tt.expectedSpanStatus, handlerSpan.Status.Code,
				"span status code mismatch: expected %v, got %v (description: %s)",
				tt.expectedSpanStatus, handlerSpan.Status.Code, handlerSpan.Status.Description)
		})
	}
}

// spanNames is a test helper that extracts span names for diagnostic messages.
func spanNames(spans []tracetest.SpanStub) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}

	return names
}
