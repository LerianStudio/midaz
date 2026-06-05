// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package extraction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// Repository UpdateStatus tests
// ---------------------------------------------------------------------------

func TestRepository_UpdateStatus(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)

	tests := []struct {
		name        string
		jobID       string
		status      string
		completedAt *time.Time
		setupMock   func(m *MockRepository)
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "success - update status to completed",
			jobID:       "job-123",
			status:      "completed",
			completedAt: &now,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateStatus(gomock.Any(), "job-123", "completed", &now).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name:        "success - update status without completedAt",
			jobID:       "job-456",
			status:      "failed",
			completedAt: nil,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateStatus(gomock.Any(), "job-456", "failed", nil).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name:        "error - job not found",
			jobID:       "nonexistent-job",
			status:      "completed",
			completedAt: &now,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateStatus(gomock.Any(), "nonexistent-job", "completed", &now).
					Return(errors.New("no extraction mapping found for job ID: nonexistent-job"))
			},
			wantErr: true,
			errMsg:  "no extraction mapping found",
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

			err := mockRepo.UpdateStatus(context.Background(), tt.jobID, tt.status, tt.completedAt)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Model conversion tests
// ---------------------------------------------------------------------------

func TestExtractionMappingMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	completedAt := now.Add(time.Hour)

	tests := []struct {
		name   string
		entity *datasource.ExtractionMapping
	}{
		{
			name: "full entity with completedAt",
			entity: &datasource.ExtractionMapping{
				JobID:       "job-001",
				ReportID:    "report-002",
				TemplateID:  "template-003",
				TenantID:    "tenant-004",
				Status:      "completed",
				CreatedAt:   now,
				CompletedAt: &completedAt,
			},
		},
		{
			name: "entity without completedAt (pending)",
			entity: &datasource.ExtractionMapping{
				JobID:      "job-005",
				ReportID:   "report-006",
				TemplateID: "template-007",
				TenantID:   "tenant-008",
				Status:     "pending",
				CreatedAt:  now,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &ExtractionMappingMongoDBModel{}
			model.FromEntity(tt.entity)

			assert.Equal(t, tt.entity.JobID, model.JobID)
			assert.Equal(t, tt.entity.ReportID, model.ReportID)
			assert.Equal(t, tt.entity.TemplateID, model.TemplateID)
			assert.Equal(t, tt.entity.TenantID, model.TenantID)
			assert.Equal(t, tt.entity.Status, model.Status)
			assert.Equal(t, tt.entity.CreatedAt, model.CreatedAt)
			assert.Equal(t, tt.entity.CompletedAt, model.CompletedAt)
		})
	}
}

func TestExtractionMappingMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	completedAt := now.Add(time.Hour)

	model := &ExtractionMappingMongoDBModel{
		JobID:       "job-123",
		ReportID:    "report-456",
		TemplateID:  "template-789",
		TenantID:    "tenant-001",
		Status:      "completed",
		CreatedAt:   now,
		CompletedAt: &completedAt,
	}

	entity := model.ToEntity()

	assert.Equal(t, model.JobID, entity.JobID)
	assert.Equal(t, model.ReportID, entity.ReportID)
	assert.Equal(t, model.TemplateID, entity.TemplateID)
	assert.Equal(t, model.TenantID, entity.TenantID)
	assert.Equal(t, model.Status, entity.Status)
	assert.Equal(t, model.CreatedAt, entity.CreatedAt)
	assert.Equal(t, model.CompletedAt, entity.CompletedAt)
}
