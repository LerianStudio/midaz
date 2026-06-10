// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	reportData "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestUseCase_ShouldSkipProcessing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	useCase := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		ReportDataRepo: mockReportDataRepo,
	}

	tests := []struct {
		name         string
		reportID     uuid.UUID
		mockSetup    func(reportID uuid.UUID)
		expectedSkip bool
	}{
		{
			name:     "Success - Skip report already finished",
			reportID: uuid.New(),
			mockSetup: func(reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "Finished",
					}, nil)
			},
			expectedSkip: true,
		},
		{
			name:     "Success - Skip report in error state",
			reportID: uuid.New(),
			mockSetup: func(reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "Error",
					}, nil)
			},
			expectedSkip: true,
		},
		{
			name:     "Success - Don't skip report still processing",
			reportID: uuid.New(),
			mockSetup: func(reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "Processing",
					}, nil)
			},
			expectedSkip: false,
		},
		{
			name:     "Success - Don't skip report not found (first attempt)",
			reportID: uuid.New(),
			mockSetup: func(reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(nil, errors.New("not found"))
			},
			expectedSkip: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup(tt.reportID)

			result := useCase.shouldSkipProcessing(context.Background(), tt.reportID)
			assert.Equal(t, tt.expectedSkip, result, "shouldSkipProcessing()")
		})
	}
}

func TestUseCase_CheckReportStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	tests := []struct {
		name           string
		reportID       uuid.UUID
		mockSetup      func(reportID uuid.UUID)
		expectedStatus string
		expectError    bool
	}{
		{
			name:     "Success - Get report status",
			reportID: uuid.New(),
			mockSetup: func(reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(&reportData.Report{
						ID:     reportID,
						Status: "Processing",
					}, nil)
			},
			expectedStatus: "Processing",
			expectError:    false,
		},
		{
			name:     "Error - Report not found",
			reportID: uuid.New(),
			mockSetup: func(reportID uuid.UUID) {
				mockReportDataRepo.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(nil, errors.New("not found"))
			},
			expectedStatus: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup(tt.reportID)

			useCase := &UseCase{
				Logger:         log.NewNop(),
				Tracer:         noop.NewTracerProvider().Tracer("test"),
				ReportDataRepo: mockReportDataRepo,
			}

			status, err := useCase.checkReportStatus(context.Background(), tt.reportID)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}
