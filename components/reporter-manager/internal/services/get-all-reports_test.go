// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_GetAllReports(t *testing.T) {
	t.Parallel()

	templateId := uuid.New()
	reportId1 := uuid.New()
	reportId2 := uuid.New()
	timeNow := time.Now()

	filters := http.QueryHeader{
		Limit:  10,
		Page:   1,
		Status: constant.FinishedStatus,
	}

	mockReports := []*report.Report{
		{
			ID:          reportId1,
			TemplateID:  templateId,
			Filters:     nil,
			Status:      constant.FinishedStatus,
			CompletedAt: &timeNow,
			CreatedAt:   timeNow,
			UpdatedAt:   timeNow,
			DeletedAt:   nil,
		},
		{
			ID:          reportId2,
			TemplateID:  templateId,
			Filters:     nil,
			Status:      constant.ProcessingStatus,
			CompletedAt: nil,
			CreatedAt:   timeNow,
			UpdatedAt:   timeNow,
			DeletedAt:   nil,
		},
	}

	tests := []struct {
		name           string
		filters        http.QueryHeader
		mockSetup      func(ctrl *gomock.Controller) *UseCase
		expectErr      bool
		expectedErr    error
		expectedResult []*report.Report
		expectedCount  int
	}{
		{
			name:    "Success - Get all reports",
			filters: filters,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockReportRepo := report.NewMockRepository(ctrl)
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(mockReports, nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ReportRepo: mockReportRepo}
			},
			expectErr:      false,
			expectedResult: mockReports,
			expectedCount:  2,
		},
		{
			name:    "Success - Get all reports with status filter",
			filters: filters,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockReportRepo := report.NewMockRepository(ctrl)
				filteredReports := []*report.Report{mockReports[0]} // Only finished reports
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(filteredReports, nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ReportRepo: mockReportRepo}
			},
			expectErr:      false,
			expectedResult: []*report.Report{mockReports[0]},
			expectedCount:  1,
		},
		{
			name:    "Error - Failed to retrieve reports",
			filters: filters,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockReportRepo := report.NewMockRepository(ctrl)
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, constant.ErrInternalServer)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ReportRepo: mockReportRepo}
			},
			expectErr:      true,
			expectedErr:    constant.ErrInternalServer,
			expectedResult: nil,
			expectedCount:  0,
		},
		{
			name:    "Success - Empty result set",
			filters: filters,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockReportRepo := report.NewMockRepository(ctrl)
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*report.Report{}, nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ReportRepo: mockReportRepo}
			},
			expectErr:      false, // Empty result set is valid, returns empty slice
			expectedResult: []*report.Report{},
			expectedCount:  0,
		},
		{
			name:    "Success - Nil result set returns empty slice",
			filters: filters,
			mockSetup: func(ctrl *gomock.Controller) *UseCase {
				mockReportRepo := report.NewMockRepository(ctrl)
				mockReportRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, nil)
				return &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test"), ReportRepo: mockReportRepo}
			},
			expectErr:      false,
			expectedResult: []*report.Report{},
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			reportSvc := tt.mockSetup(ctrl)

			ctx := context.Background()
			result, err := reportSvc.GetAllReports(ctx, tt.filters)

			if tt.expectErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result, tt.expectedCount)
				if tt.expectedCount > 0 {
					assert.Equal(t, tt.expectedResult[0].ID, result[0].ID)
					assert.Equal(t, tt.expectedResult[0].Status, result[0].Status)
				}
			}
		})
	}
}
