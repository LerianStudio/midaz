// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	reportData "github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/seaweedfs/template"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestUseCase_LoadTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		templateContent []byte
		templateErr     error
		expectError     bool
	}{
		{
			name:            "Success - loads template content",
			templateContent: []byte("Hello {{ name }}"),
			templateErr:     nil,
			expectError:     false,
		},
		{
			name:            "Error - template not found",
			templateContent: nil,
			templateErr:     errors.New("template not found"),
			expectError:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTemplateRepo := template.NewMockRepository(ctrl)
			_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background()) //nolint:dogsled // only tracer needed
			_, span := tracer.Start(context.Background(), "test")

			templateID := uuid.New()
			reportID := uuid.New()

			mockTemplateRepo.EXPECT().
				Get(gomock.Any(), templateID.String()).
				Return(tt.templateContent, tt.templateErr)

			useCase := &UseCase{
				Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
				TemplateSeaweedFS: mockTemplateRepo,
			}

			if tt.expectError {
				mockReportDataRepo := reportData.NewMockRepository(ctrl)
				mockReportDataRepo.EXPECT().
					UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
					Return(nil)
				useCase.ReportDataRepo = mockReportDataRepo
			}

			message := GenerateReportMessage{
				TemplateID: templateID,
				ReportID:   reportID,
			}

			result, err := useCase.loadTemplate(context.Background(), message, &span)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, string(tt.templateContent), string(result))
			}
		})
	}
}

func TestUseCase_RenderTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Success - renders template with data", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background()) //nolint:dogsled // only tracer needed
		_, span := tracer.Start(context.Background(), "test")

		useCase := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

		templateBytes := []byte("Hello {{ db.users.0.name }}")
		data := map[string]map[string][]map[string]any{
			"db": {
				"users": {
					{"name": "World"},
				},
			},
		}

		message := GenerateReportMessage{
			TemplateID: uuid.New(),
			ReportID:   uuid.New(),
		}

		result, err := useCase.renderTemplate(context.Background(), templateBytes, data, message, &span)
		require.NoError(t, err)
		assert.Equal(t, "Hello World", result)
	})

	t.Run("Error - template rendering fails and report update succeeds", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockReportDataRepo := reportData.NewMockRepository(ctrl)
		_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background()) //nolint:dogsled // only tracer needed
		_, span := tracer.Start(context.Background(), "test")

		reportID := uuid.New()

		// updateReportWithErrors succeeds, so renderTemplate returns the original rendering error
		mockReportDataRepo.EXPECT().
			UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
			Return(nil)

		useCase := &UseCase{
			Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
			ReportDataRepo: mockReportDataRepo,
		}

		// Invalid pongo2 template syntax triggers rendering error
		templateBytes := []byte("Hello {{ name !")
		data := map[string]map[string][]map[string]any{}

		message := GenerateReportMessage{
			TemplateID: uuid.New(),
			ReportID:   reportID,
		}

		_, err := useCase.renderTemplate(context.Background(), templateBytes, data, message, &span)
		require.Error(t, err)
	})

	t.Run("Error - template rendering fails and report update also fails", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockReportDataRepo := reportData.NewMockRepository(ctrl)
		_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background()) //nolint:dogsled // only tracer needed
		_, span := tracer.Start(context.Background(), "test")

		reportID := uuid.New()

		// updateReportWithErrors also fails, so renderTemplate returns the update error
		mockReportDataRepo.EXPECT().
			UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
			Return(errors.New("database unavailable"))

		useCase := &UseCase{
			Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
			ReportDataRepo: mockReportDataRepo,
		}

		// Invalid pongo2 template syntax triggers rendering error
		templateBytes := []byte("Hello {{ name !")
		data := map[string]map[string][]map[string]any{}

		message := GenerateReportMessage{
			TemplateID: uuid.New(),
			ReportID:   reportID,
		}

		_, err := useCase.renderTemplate(context.Background(), templateBytes, data, message, &span)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database unavailable")
	})
}

func TestUseCase_LoadTemplate_UpdateReportAlsoFails(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background()) //nolint:dogsled // only tracer needed
	_, span := tracer.Start(context.Background(), "test")

	templateID := uuid.New()
	reportID := uuid.New()

	mockTemplateRepo.EXPECT().
		Get(gomock.Any(), templateID.String()).
		Return(nil, errors.New("template not found"))

	// updateReportWithErrors also fails
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), "Error", reportID, gomock.Any(), gomock.Any()).
		Return(errors.New("db connection lost"))

	useCase := &UseCase{
		Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
		TemplateSeaweedFS: mockTemplateRepo,
		ReportDataRepo:    mockReportDataRepo,
	}

	message := GenerateReportMessage{
		TemplateID: templateID,
		ReportID:   reportID,
	}

	_, err := useCase.loadTemplate(context.Background(), message, &span)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db connection lost")
}

// NOTE: Standalone test for the non-PDF early-return path. The PDF conversion path requires a live
// chromedp worker pool (headless Chrome), making it an integration-level concern tested under
// tests/integration/. A table-driven test combining both paths is not feasible at the unit level.
func TestUseCase_ConvertToPDFIfNeeded_NonPDFFormat(t *testing.T) {
	t.Parallel()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background()) //nolint:dogsled // only tracer needed
	_, span := tracer.Start(context.Background(), "test")

	useCase := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}

	message := GenerateReportMessage{
		ReportID:     uuid.New(),
		OutputFormat: "html",
	}

	htmlContent := "<html><body>Test</body></html>"

	result, err := useCase.convertToPDFIfNeeded(context.Background(), message, htmlContent, &span)
	require.NoError(t, err)
	assert.Equal(t, htmlContent, result, "expected unchanged content for non-PDF format")
}
