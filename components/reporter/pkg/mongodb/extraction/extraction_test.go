// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package extraction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/datasource"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// Repository interface contract tests (gomock-based)
// ---------------------------------------------------------------------------

func TestRepository_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)

	tests := []struct {
		name      string
		mapping   *datasource.ExtractionMapping
		setupMock func(m *MockRepository)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success - create extraction mapping",
			mapping: &datasource.ExtractionMapping{
				JobID:      "job-123",
				ReportID:   "report-456",
				TemplateID: "template-789",
				TenantID:   "tenant-001",
				Status:     "pending",
				CreatedAt:  now,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "error - database insertion fails",
			mapping: &datasource.ExtractionMapping{
				JobID:      "job-fail",
				ReportID:   "report-fail",
				TemplateID: "template-fail",
				TenantID:   "tenant-fail",
				Status:     "pending",
				CreatedAt:  now,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(errors.New("connection refused"))
			},
			wantErr: true,
			errMsg:  "connection refused",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			err := mockRepo.Create(context.Background(), tt.mapping)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRepository_FindByJobID(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)

	validMapping := &datasource.ExtractionMapping{
		JobID:      "job-123",
		ReportID:   "report-456",
		TemplateID: "template-789",
		TenantID:   "tenant-001",
		Status:     "pending",
		CreatedAt:  now,
	}

	tests := []struct {
		name        string
		jobID       string
		setupMock   func(m *MockRepository)
		wantMapping *datasource.ExtractionMapping
		wantErr     bool
		errMsg      string
	}{
		{
			name:  "success - mapping found",
			jobID: "job-123",
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByJobID(gomock.Any(), "job-123").
					Return(validMapping, nil)
			},
			wantMapping: validMapping,
			wantErr:     false,
		},
		{
			name:  "error - mapping not found",
			jobID: "nonexistent-job",
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByJobID(gomock.Any(), "nonexistent-job").
					Return(nil, errors.New("mongo: no documents in result"))
			},
			wantMapping: nil,
			wantErr:     true,
			errMsg:      "no documents",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			result, err := mockRepo.FindByJobID(context.Background(), tt.jobID)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantMapping.JobID, result.JobID)
				assert.Equal(t, tt.wantMapping.ReportID, result.ReportID)
			}
		})
	}
}

func TestRepository_FindByReportID(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)

	validMapping := &datasource.ExtractionMapping{
		JobID:      "job-123",
		ReportID:   "report-456",
		TemplateID: "template-789",
		TenantID:   "tenant-001",
		Status:     "completed",
		CreatedAt:  now,
	}

	tests := []struct {
		name        string
		reportID    string
		setupMock   func(m *MockRepository)
		wantMapping *datasource.ExtractionMapping
		wantErr     bool
		errMsg      string
	}{
		{
			name:     "success - mapping found by report ID",
			reportID: "report-456",
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByReportID(gomock.Any(), "report-456").
					Return(validMapping, nil)
			},
			wantMapping: validMapping,
			wantErr:     false,
		},
		{
			name:     "error - no mapping for report",
			reportID: "nonexistent-report",
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByReportID(gomock.Any(), "nonexistent-report").
					Return(nil, errors.New("mongo: no documents in result"))
			},
			wantMapping: nil,
			wantErr:     true,
			errMsg:      "no documents",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			result, err := mockRepo.FindByReportID(context.Background(), tt.reportID)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantMapping.ReportID, result.ReportID)
			}
		})
	}
}
