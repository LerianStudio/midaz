// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/seaweedfs/report"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestUseCase_GetContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		extension    string
		expectedType string
	}{
		{
			name:         "Success - existing mime type",
			extension:    "html",
			expectedType: "text/html",
		},
		{
			name:         "Success - pdf mime type",
			extension:    "pdf",
			expectedType: "application/pdf",
		},
		{
			name:         "Success - unknown mime type",
			extension:    "unknown",
			expectedType: "text/plain",
		},
		{
			name:         "Success - empty extension",
			extension:    "",
			expectedType: "text/plain",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := getContentType(tt.extension)
			assert.Equal(t, tt.expectedType, got, "getContentType(%q)", tt.extension)
		})
	}
}

func TestUseCase_SaveReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		outputFormat   string
		renderedOutput string
		reportTTL      string
		expectedType   string
		putErr         error
		expectError    bool
		errContains    string
	}{
		{
			name:           "Success - saves CSV report",
			outputFormat:   "csv",
			renderedOutput: "id,name\n1,Jane",
			expectedType:   "text/csv",
			putErr:         nil,
			expectError:    false,
		},
		{
			name:           "Error - Put fails",
			outputFormat:   "html",
			renderedOutput: "<html></html>",
			expectedType:   "text/html",
			putErr:         errors.New("failed to put file"),
			expectError:    true,
			errContains:    "failed to put file",
		},
		{
			name:           "Success - saves JSON report with TTL",
			outputFormat:   "json",
			renderedOutput: `{"data": "test"}`,
			reportTTL:      "30d",
			expectedType:   "application/json",
			putErr:         nil,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockReportRepo := report.NewMockRepository(ctrl)

			useCase := &UseCase{
				Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"),
				ReportSeaweedFS: mockReportRepo,
				ReportTTL:       tt.reportTTL,
			}

			reportID := uuid.New()
			message := GenerateReportMessage{
				ReportID:     reportID,
				TemplateID:   uuid.New(),
				OutputFormat: tt.outputFormat,
			}

			mockReportRepo.
				EXPECT().
				Put(gomock.Any(), gomock.Any(), tt.expectedType, gomock.Any(), tt.reportTTL).
				Return(tt.putErr)

			ctx := context.Background()
			err := useCase.saveReport(ctx, message, tt.renderedOutput)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
