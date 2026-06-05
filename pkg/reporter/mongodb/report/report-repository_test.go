// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package report

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// Repository interface contract tests (gomock-based)
// ---------------------------------------------------------------------------

func TestRepository_FindByID(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	completedAt := now.Add(time.Hour)
	reportID := uuid.New()
	templateID := uuid.New()

	validReport := &Report{
		ID:          reportID,
		TemplateID:  templateID,
		Status:      constant.FinishedStatus,
		Filters:     nil,
		Metadata:    map[string]any{"pages": 5},
		CompletedAt: &completedAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	errNotFound := errors.New("mongo: no documents in result")
	errGeneric := errors.New("connection refused")

	tests := []struct {
		name       string
		id         uuid.UUID
		setupMock  func(m *MockRepository)
		wantReport *Report
		wantErr    bool
		errMsg     string
	}{
		{
			name: "success - report found",
			id:   reportID,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(validReport, nil).
					Times(1)
			},
			wantReport: validReport,
			wantErr:    false,
		},
		{
			name: "not found - returns error",
			id:   uuid.New(),
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(nil, errNotFound).
					Times(1)
			},
			wantReport: nil,
			wantErr:    true,
			errMsg:     "no documents",
		},
		{
			name: "generic error - database failure",
			id:   reportID,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindByID(gomock.Any(), reportID).
					Return(nil, errGeneric).
					Times(1)
			},
			wantReport: nil,
			wantErr:    true,
			errMsg:     "connection refused",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			got, err := mockRepo.FindByID(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.wantReport.ID, got.ID)
				assert.Equal(t, tt.wantReport.TemplateID, got.TemplateID)
				assert.Equal(t, tt.wantReport.Status, got.Status)
				assert.Equal(t, tt.wantReport.CompletedAt, got.CompletedAt)
				assert.Equal(t, tt.wantReport.Metadata, got.Metadata)
			}
		})
	}
}

func TestRepository_FindList(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	templateID := uuid.New()

	report1 := &Report{
		ID:         uuid.New(),
		TemplateID: templateID,
		Status:     constant.FinishedStatus,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	report2 := &Report{
		ID:         uuid.New(),
		TemplateID: templateID,
		Status:     constant.ProcessingStatus,
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now.Add(-time.Hour),
	}

	errGeneric := errors.New("cursor iteration failed")

	tests := []struct {
		name        string
		filters     http.QueryHeader
		setupMock   func(m *MockRepository)
		wantReports []*Report
		wantErr     bool
		errMsg      string
	}{
		{
			name: "success - returns multiple results",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindList(gomock.Any(), http.QueryHeader{Limit: 10, Page: 1}).
					Return([]*Report{report1, report2}, nil).
					Times(1)
			},
			wantReports: []*Report{report1, report2},
			wantErr:     false,
		},
		{
			name: "success - empty results",
			filters: http.QueryHeader{
				Limit:  10,
				Page:   1,
				Status: "nonexistent",
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindList(gomock.Any(), http.QueryHeader{Limit: 10, Page: 1, Status: "nonexistent"}).
					Return([]*Report{}, nil).
					Times(1)
			},
			wantReports: []*Report{},
			wantErr:     false,
		},
		{
			name: "success - with status filter",
			filters: http.QueryHeader{
				Limit:  10,
				Page:   1,
				Status: constant.FinishedStatus,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindList(gomock.Any(), http.QueryHeader{Limit: 10, Page: 1, Status: constant.FinishedStatus}).
					Return([]*Report{report1}, nil).
					Times(1)
			},
			wantReports: []*Report{report1},
			wantErr:     false,
		},
		{
			name: "success - with template_id filter",
			filters: http.QueryHeader{
				Limit:      10,
				Page:       1,
				TemplateID: templateID,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindList(gomock.Any(), http.QueryHeader{Limit: 10, Page: 1, TemplateID: templateID}).
					Return([]*Report{report1, report2}, nil).
					Times(1)
			},
			wantReports: []*Report{report1, report2},
			wantErr:     false,
		},
		{
			name: "success - with created_at filter",
			filters: http.QueryHeader{
				Limit:     10,
				Page:      1,
				CreatedAt: now.Truncate(24 * time.Hour),
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*Report{report1}, nil).
					Times(1)
			},
			wantReports: []*Report{report1},
			wantErr:     false,
		},
		{
			name: "error - database failure",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					FindList(gomock.Any(), http.QueryHeader{Limit: 10, Page: 1}).
					Return(nil, errGeneric).
					Times(1)
			},
			wantReports: nil,
			wantErr:     true,
			errMsg:      "cursor iteration failed",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			got, err := mockRepo.FindList(context.Background(), tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Len(t, got, len(tt.wantReports))

				for i, want := range tt.wantReports {
					assert.Equal(t, want.ID, got[i].ID)
					assert.Equal(t, want.Status, got[i].Status)
					assert.Equal(t, want.TemplateID, got[i].TemplateID)
				}
			}
		})
	}
}

func TestRepository_Create(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	reportID := uuid.New()
	templateID := uuid.New()

	filters := map[string]map[string]map[string]model.FilterCondition{
		"transactions": {
			"amount": {
				"range": {Between: []any{100, 500}},
			},
		},
	}

	inputReport := &Report{
		ID:         reportID,
		TemplateID: templateID,
		Status:     constant.ProcessingStatus,
		Filters:    filters,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	createdReport := &Report{
		ID:         reportID,
		TemplateID: templateID,
		Status:     constant.ProcessingStatus,
		Filters:    filters,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	errDuplicate := errors.New("duplicate key error")

	tests := []struct {
		name       string
		input      *Report
		setupMock  func(m *MockRepository)
		wantReport *Report
		wantErr    bool
		errMsg     string
	}{
		{
			name:  "success - report created",
			input: inputReport,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					Create(gomock.Any(), inputReport).
					Return(createdReport, nil).
					Times(1)
			},
			wantReport: createdReport,
			wantErr:    false,
		},
		{
			name:  "error - duplicate key",
			input: inputReport,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					Create(gomock.Any(), inputReport).
					Return(nil, errDuplicate).
					Times(1)
			},
			wantReport: nil,
			wantErr:    true,
			errMsg:     "duplicate key",
		},
		{
			name:  "success - report created with nil filters",
			input: &Report{ID: reportID, TemplateID: templateID, Status: constant.ProcessingStatus},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&Report{
						ID:         reportID,
						TemplateID: templateID,
						Status:     constant.ProcessingStatus,
						CreatedAt:  now,
						UpdatedAt:  now,
					}, nil).
					Times(1)
			},
			wantReport: &Report{
				ID:         reportID,
				TemplateID: templateID,
				Status:     constant.ProcessingStatus,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			got, err := mockRepo.Create(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.wantReport.ID, got.ID)
				assert.Equal(t, tt.wantReport.TemplateID, got.TemplateID)
				assert.Equal(t, tt.wantReport.Status, got.Status)
				assert.False(t, got.CreatedAt.IsZero())
				assert.False(t, got.UpdatedAt.IsZero())
			}
		})
	}
}

func TestRepository_UpdateReportStatusById(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	reportID := uuid.New()

	errNotFound := errors.New("no report found with the provided UUID")

	tests := []struct {
		name        string
		status      string
		id          uuid.UUID
		completedAt time.Time
		metadata    map[string]any
		setupMock   func(m *MockRepository)
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "success - update to Finished status",
			status:      constant.FinishedStatus,
			id:          reportID,
			completedAt: now,
			metadata:    map[string]any{"pages": 12, "fileSize": 1024},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateReportStatusById(
						gomock.Any(),
						constant.FinishedStatus,
						reportID,
						now,
						map[string]any{"pages": 12, "fileSize": 1024},
					).
					Return(nil).
					Times(1)
			},
			wantErr: false,
		},
		{
			name:        "success - update to Error status with error metadata",
			status:      constant.ErrorStatus,
			id:          reportID,
			completedAt: now,
			metadata: map[string]any{
				"error":   "template rendering failed",
				"retries": 3,
			},
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateReportStatusById(
						gomock.Any(),
						constant.ErrorStatus,
						reportID,
						now,
						map[string]any{
							"error":   "template rendering failed",
							"retries": 3,
						},
					).
					Return(nil).
					Times(1)
			},
			wantErr: false,
		},
		{
			name:        "success - update with nil metadata",
			status:      constant.FinishedStatus,
			id:          reportID,
			completedAt: now,
			metadata:    nil,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateReportStatusById(
						gomock.Any(),
						constant.FinishedStatus,
						reportID,
						now,
						nil,
					).
					Return(nil).
					Times(1)
			},
			wantErr: false,
		},
		{
			name:        "success - update with zero completedAt",
			status:      constant.ProcessingStatus,
			id:          reportID,
			completedAt: time.Time{},
			metadata:    nil,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateReportStatusById(
						gomock.Any(),
						constant.ProcessingStatus,
						reportID,
						time.Time{},
						nil,
					).
					Return(nil).
					Times(1)
			},
			wantErr: false,
		},
		{
			name:        "error - report not found",
			status:      constant.FinishedStatus,
			id:          uuid.New(),
			completedAt: now,
			metadata:    nil,
			setupMock: func(m *MockRepository) {
				m.EXPECT().
					UpdateReportStatusById(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
					).
					Return(errNotFound).
					Times(1)
			},
			wantErr: true,
			errMsg:  "no report found",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockRepo := NewMockRepository(ctrl)
			tt.setupMock(mockRepo)

			err := mockRepo.UpdateReportStatusById(
				context.Background(),
				tt.status,
				tt.id,
				tt.completedAt,
				tt.metadata,
			)

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
// Domain entity tests: NewReport constructor
// ---------------------------------------------------------------------------

func TestNewReport_Validation(t *testing.T) {
	t.Parallel()

	validID := uuid.New()
	validTemplateID := uuid.New()
	validFilters := map[string]map[string]map[string]model.FilterCondition{
		"transactions": {
			"amount": {
				"eq": {Equals: []any{100}},
			},
		},
	}

	tests := []struct {
		name        string
		id          uuid.UUID
		templateID  uuid.UUID
		status      string
		filters     map[string]map[string]map[string]model.FilterCondition
		wantErr     bool
		expectedErr error
	}{
		{
			name:       "valid - all fields populated",
			id:         validID,
			templateID: validTemplateID,
			status:     constant.ProcessingStatus,
			filters:    validFilters,
			wantErr:    false,
		},
		{
			name:       "valid - nil filters are allowed",
			id:         validID,
			templateID: validTemplateID,
			status:     constant.FinishedStatus,
			filters:    nil,
			wantErr:    false,
		},
		{
			name:        "invalid - nil ID",
			id:          uuid.Nil,
			templateID:  validTemplateID,
			status:      constant.ProcessingStatus,
			filters:     validFilters,
			wantErr:     true,
			expectedErr: constant.ErrMissingRequiredFields,
		},
		{
			name:        "invalid - nil templateID",
			id:          validID,
			templateID:  uuid.Nil,
			status:      constant.ProcessingStatus,
			filters:     validFilters,
			wantErr:     true,
			expectedErr: constant.ErrMissingRequiredFields,
		},
		{
			name:        "invalid - empty status",
			id:          validID,
			templateID:  validTemplateID,
			status:      "",
			filters:     validFilters,
			wantErr:     true,
			expectedErr: constant.ErrMissingRequiredFields,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewReport(tt.id, tt.templateID, tt.status, tt.filters, "xml", "Test Template")

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)

				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr),
						"expected error wrapping %v, got %v", tt.expectedErr, err)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.id, got.ID)
				assert.Equal(t, tt.templateID, got.TemplateID)
				assert.Equal(t, tt.status, got.Status)
				assert.Equal(t, tt.filters, got.Filters)
				assert.False(t, got.CreatedAt.IsZero(), "CreatedAt must be set")
				assert.False(t, got.UpdatedAt.IsZero(), "UpdatedAt must be set")
				assert.Nil(t, got.Metadata, "Metadata should be nil on new report")
				assert.Nil(t, got.CompletedAt, "CompletedAt should be nil on new report")
				assert.Nil(t, got.DeletedAt, "DeletedAt should be nil on new report")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Domain entity tests: ReconstructReport
// ---------------------------------------------------------------------------

func TestReconstructReport_BypassesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          uuid.UUID
		templateID  uuid.UUID
		status      string
		filters     map[string]map[string]map[string]model.FilterCondition
		metadata    map[string]any
		completedAt *time.Time
		createdAt   time.Time
		updatedAt   time.Time
		deletedAt   *time.Time
	}{
		{
			name:       "zero values pass without error",
			id:         uuid.Nil,
			templateID: uuid.Nil,
			status:     "",
			createdAt:  time.Time{},
			updatedAt:  time.Time{},
		},
		{
			name:       "full reconstruction from database",
			id:         uuid.New(),
			templateID: uuid.New(),
			status:     constant.FinishedStatus,
			filters: map[string]map[string]map[string]model.FilterCondition{
				"t": {"c": {"f": {Equals: []any{"v"}}}},
			},
			metadata:    map[string]any{"key": "val"},
			completedAt: timePtr(time.Now()),
			createdAt:   time.Now().Add(-time.Hour),
			updatedAt:   time.Now(),
			deletedAt:   nil,
		},
		{
			name:       "soft-deleted record",
			id:         uuid.New(),
			templateID: uuid.New(),
			status:     constant.ErrorStatus,
			deletedAt:  timePtr(time.Now()),
			createdAt:  time.Now().Add(-2 * time.Hour),
			updatedAt:  time.Now().Add(-time.Hour),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ReconstructReport(
				tt.id, tt.templateID, tt.status, tt.filters,
				tt.metadata, tt.completedAt, tt.createdAt, tt.updatedAt, tt.deletedAt, "", "",
			)

			require.NotNil(t, got, "ReconstructReport must never return nil")
			assert.Equal(t, tt.id, got.ID)
			assert.Equal(t, tt.templateID, got.TemplateID)
			assert.Equal(t, tt.status, got.Status)
			assert.Equal(t, tt.filters, got.Filters)
			assert.Equal(t, tt.metadata, got.Metadata)
			assert.Equal(t, tt.completedAt, got.CompletedAt)
			assert.Equal(t, tt.createdAt, got.CreatedAt)
			assert.Equal(t, tt.updatedAt, got.UpdatedAt)
			assert.Equal(t, tt.deletedAt, got.DeletedAt)
		})
	}
}

// ---------------------------------------------------------------------------
// Domain entity tests: ToEntity / FromEntity conversions
// ---------------------------------------------------------------------------

func TestReportMongoDBModel_ToEntity_Conversion(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	completedAt := now.Add(time.Hour)
	id := uuid.New()
	templateID := uuid.New()

	customFilters := map[string]map[string]map[string]model.FilterCondition{
		"accounts": {
			"balance": {
				"gte": {GreaterOrEqual: []any{1000}},
			},
		},
	}

	tests := []struct {
		name       string
		model      *ReportMongoDBModel
		filters    map[string]map[string]map[string]model.FilterCondition
		wantID     uuid.UUID
		wantStatus string
	}{
		{
			name: "full model with external filters override",
			model: &ReportMongoDBModel{
				ID:          id,
				TemplateID:  templateID,
				Status:      constant.FinishedStatus,
				Filters:     nil,
				CompletedAt: &completedAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			filters:    customFilters,
			wantID:     id,
			wantStatus: constant.FinishedStatus,
		},
		{
			name: "model with nil filters passed as nil",
			model: &ReportMongoDBModel{
				ID:         id,
				TemplateID: templateID,
				Status:     constant.ProcessingStatus,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			filters:    nil,
			wantID:     id,
			wantStatus: constant.ProcessingStatus,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := tt.model.ToEntity(tt.filters)

			require.NotNil(t, entity)
			assert.Equal(t, tt.wantID, entity.ID)
			assert.Equal(t, tt.wantStatus, entity.Status)
			assert.Equal(t, tt.filters, entity.Filters)
			// ToEntity passes nil for metadata per implementation
			assert.Nil(t, entity.Metadata)
		})
	}
}

func TestReportMongoDBModel_ToEntityFindByID_Conversion(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Millisecond)
	completedAt := now.Add(time.Hour)
	deletedAt := now.Add(2 * time.Hour)
	id := uuid.New()
	templateID := uuid.New()

	storedFilters := map[string]map[string]map[string]model.FilterCondition{
		"orders": {
			"total": {
				"between": {Between: []any{100, 5000}},
			},
		},
	}

	metadata := map[string]any{
		"generatedBy": "worker-1",
		"pages":       10,
	}

	mongoModel := &ReportMongoDBModel{
		ID:          id,
		TemplateID:  templateID,
		Status:      constant.FinishedStatus,
		Filters:     storedFilters,
		Metadata:    metadata,
		CompletedAt: &completedAt,
		CreatedAt:   now,
		UpdatedAt:   now,
		DeletedAt:   &deletedAt,
	}

	entity := mongoModel.ToEntityFindByID()

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID)
	assert.Equal(t, templateID, entity.TemplateID)
	assert.Equal(t, constant.FinishedStatus, entity.Status)
	assert.Equal(t, storedFilters, entity.Filters, "ToEntityFindByID must use stored filters")
	assert.Equal(t, metadata, entity.Metadata, "ToEntityFindByID must preserve metadata")
	assert.Equal(t, &completedAt, entity.CompletedAt)
	assert.Equal(t, now, entity.CreatedAt)
	assert.Equal(t, now, entity.UpdatedAt)
	assert.Equal(t, &deletedAt, entity.DeletedAt)
}

func TestReportMongoDBModel_FromEntity_Conversion(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	templateID := uuid.New()
	completedAt := time.Now().Truncate(time.Millisecond)

	filters := map[string]map[string]map[string]model.FilterCondition{
		"ledger": {
			"currency": {
				"in": {In: []any{"USD", "EUR", "BRL"}},
			},
		},
	}

	tests := []struct {
		name  string
		input *Report
	}{
		{
			name: "full report with all fields",
			input: &Report{
				ID:          id,
				TemplateID:  templateID,
				Status:      constant.FinishedStatus,
				Filters:     filters,
				Metadata:    map[string]any{"key": "value"},
				CompletedAt: &completedAt,
			},
		},
		{
			name: "minimal report without optional fields",
			input: &Report{
				ID:         id,
				TemplateID: templateID,
				Status:     constant.ProcessingStatus,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mongoModel := &ReportMongoDBModel{}
			err := mongoModel.FromEntity(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.input.ID, mongoModel.ID)
			assert.Equal(t, tt.input.TemplateID, mongoModel.TemplateID)
			assert.Equal(t, tt.input.Status, mongoModel.Status)
			assert.Equal(t, tt.input.Filters, mongoModel.Filters)
			assert.Equal(t, tt.input.Metadata, mongoModel.Metadata)
			assert.Equal(t, tt.input.CompletedAt, mongoModel.CompletedAt)
			assert.False(t, mongoModel.CreatedAt.IsZero(), "FromEntity must set CreatedAt to now")
			assert.False(t, mongoModel.UpdatedAt.IsZero(), "FromEntity must set UpdatedAt to now")
			assert.Nil(t, mongoModel.DeletedAt, "FromEntity must set DeletedAt to nil")
		})
	}
}

// ---------------------------------------------------------------------------
// Round-trip correctness tests
// ---------------------------------------------------------------------------

func TestRoundTrip_FromEntity_ToEntityFindByID(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	templateID := uuid.New()

	filters := map[string]map[string]map[string]model.FilterCondition{
		"transactions": {
			"amount": {
				"gte": {GreaterOrEqual: []any{500}},
			},
			"status": {
				"in": {In: []any{"completed", "pending"}},
			},
		},
	}

	original := &Report{
		ID:         id,
		TemplateID: templateID,
		Status:     constant.ProcessingStatus,
		Filters:    filters,
		Metadata:   map[string]any{"source": "api"},
	}

	// Step 1: Convert entity to MongoDB model
	mongoModel := &ReportMongoDBModel{}
	err := mongoModel.FromEntity(original)
	require.NoError(t, err)

	// Step 2: Convert back to entity
	roundTripped := mongoModel.ToEntityFindByID()
	require.NotNil(t, roundTripped)

	// Step 3: Verify identity fields are preserved
	assert.Equal(t, original.ID, roundTripped.ID, "ID must survive round-trip")
	assert.Equal(t, original.TemplateID, roundTripped.TemplateID, "TemplateID must survive round-trip")
	assert.Equal(t, original.Status, roundTripped.Status, "Status must survive round-trip")
	assert.Equal(t, original.Filters, roundTripped.Filters, "Filters must survive round-trip")
	assert.Equal(t, original.Metadata, roundTripped.Metadata, "Metadata must survive round-trip")

	// Timestamps are reset by FromEntity, so we only check they are non-zero
	assert.False(t, roundTripped.CreatedAt.IsZero(), "CreatedAt must be set after round-trip")
	assert.False(t, roundTripped.UpdatedAt.IsZero(), "UpdatedAt must be set after round-trip")
	assert.Nil(t, roundTripped.DeletedAt, "DeletedAt must be nil after round-trip")
}

func TestRoundTrip_FromEntity_ToEntity_WithExternalFilters(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	templateID := uuid.New()

	storedFilters := map[string]map[string]map[string]model.FilterCondition{
		"original": {"col": {"f": {Equals: []any{"stored"}}}},
	}

	externalFilters := map[string]map[string]map[string]model.FilterCondition{
		"external": {"col": {"f": {Equals: []any{"overridden"}}}},
	}

	original := &Report{
		ID:         id,
		TemplateID: templateID,
		Status:     constant.ProcessingStatus,
		Filters:    storedFilters,
	}

	mongoModel := &ReportMongoDBModel{}
	err := mongoModel.FromEntity(original)
	require.NoError(t, err)

	// ToEntity uses the externally provided filters, not the stored ones
	entity := mongoModel.ToEntity(externalFilters)

	assert.Equal(t, externalFilters, entity.Filters,
		"ToEntity must use the externally provided filters, not stored ones")
	assert.Nil(t, entity.Metadata,
		"ToEntity passes nil for metadata")
}

// ---------------------------------------------------------------------------
// MockRepository satisfies the Repository interface compile-time check
// ---------------------------------------------------------------------------

func TestMockRepository_ImplementsInterface(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mock := NewMockRepository(ctrl)

	// Compile-time assertion that MockRepository satisfies Repository
	var _ Repository = mock

	assert.NotNil(t, mock)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func timePtr(t time.Time) *time.Time {
	return &t
}
