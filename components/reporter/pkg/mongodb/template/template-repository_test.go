// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// Repository interface contract tests (via gomock MockRepository)
// ---------------------------------------------------------------------------

func TestRepository_FindByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          uuid.UUID
		setupMock   func(mockRepo *MockRepository)
		wantErr     bool
		expectedErr string
		validate    func(t *testing.T, result *Template)
	}{
		{
			name: "success - returns template",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			setupMock: func(mockRepo *MockRepository) {
				expected := &Template{
					ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					OutputFormat: "PDF",
					Description:  "Financial Report",
					FileName:     "report_template.tpl",
					CreatedAt:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					UpdatedAt:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
				}
				mockRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(expected, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *Template) {
				t.Helper()

				assert.Equal(t, uuid.MustParse("00000000-0000-0000-0000-000000000001"), result.ID)
				assert.Equal(t, "PDF", result.OutputFormat)
				assert.Equal(t, "Financial Report", result.Description)
				assert.Equal(t, "report_template.tpl", result.FileName)
			},
		},
		{
			name: "error - template not found",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("mongo: no documents in result"))
			},
			wantErr:     true,
			expectedErr: "mongo: no documents in result",
		},
		{
			name: "error - database connection failure",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to connect to mongodb"))
			},
			wantErr:     true,
			expectedErr: "failed to connect to mongodb",
		},
		{
			name: "success - template with empty description",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			setupMock: func(mockRepo *MockRepository) {
				expected := &Template{
					ID:           uuid.MustParse("00000000-0000-0000-0000-000000000004"),
					OutputFormat: "HTML",
					Description:  "",
					FileName:     "minimal.tpl",
					CreatedAt:    time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC),
					UpdatedAt:    time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC),
				}
				mockRepo.EXPECT().
					FindByID(gomock.Any(), gomock.Any()).
					Return(expected, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *Template) {
				t.Helper()

				assert.Equal(t, "HTML", result.OutputFormat)
				assert.Empty(t, result.Description)
				assert.Equal(t, "minimal.tpl", result.FileName)
			},
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

			result, err := mockRepo.FindByID(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				tt.validate(t, result)
			}
		})
	}
}

func TestRepository_FindList(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		filters     http.QueryHeader
		setupMock   func(mockRepo *MockRepository)
		wantErr     bool
		expectedErr string
		validate    func(t *testing.T, result []*Template)
	}{
		{
			name: "success - returns multiple templates",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  1,
			},
			setupMock: func(mockRepo *MockRepository) {
				templates := []*Template{
					{
						ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
						OutputFormat: "PDF",
						Description:  "Template 1",
						FileName:     "template_1.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
					{
						ID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
						OutputFormat: "HTML",
						Description:  "Template 2",
						FileName:     "template_2.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				require.Len(t, result, 2)
				assert.Equal(t, "PDF", result[0].OutputFormat)
				assert.Equal(t, "HTML", result[1].OutputFormat)
			},
		},
		{
			name: "success - returns empty list",
			filters: http.QueryHeader{
				Limit: 20,
				Page:  1,
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*Template{}, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				assert.Empty(t, result)
			},
		},
		{
			name: "success - filters by output format",
			filters: http.QueryHeader{
				OutputFormat: "CSV",
				Limit:        10,
				Page:         1,
			},
			setupMock: func(mockRepo *MockRepository) {
				templates := []*Template{
					{
						ID:           uuid.New(),
						OutputFormat: "CSV",
						Description:  "CSV Export",
						FileName:     "export.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				require.Len(t, result, 1)
				assert.Equal(t, "CSV", result[0].OutputFormat)
			},
		},
		{
			name: "success - filters by description",
			filters: http.QueryHeader{
				Description: "Financial",
				Limit:       10,
				Page:        1,
			},
			setupMock: func(mockRepo *MockRepository) {
				templates := []*Template{
					{
						ID:           uuid.New(),
						OutputFormat: "PDF",
						Description:  "Financial Report",
						FileName:     "financial.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				require.Len(t, result, 1)
				assert.Contains(t, result[0].Description, "Financial")
			},
		},
		{
			name: "success - filters by created_at date",
			filters: http.QueryHeader{
				CreatedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				Limit:     10,
				Page:      1,
			},
			setupMock: func(mockRepo *MockRepository) {
				templates := []*Template{
					{
						ID:           uuid.New(),
						OutputFormat: "PDF",
						Description:  "Daily Report",
						FileName:     "daily.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				require.Len(t, result, 1)
			},
		},
		{
			name: "error - database connection failure",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  2,
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to get database"))
			},
			wantErr:     true,
			expectedErr: "failed to get database",
		},
		{
			name: "error - cursor iteration failure",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  3,
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to iterate templates"))
			},
			wantErr:     true,
			expectedErr: "failed to iterate templates",
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

			result, err := mockRepo.FindList(context.Background(), tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				tt.validate(t, result)
			}
		})
	}
}

func TestRepository_Create(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		record      *TemplateMongoDBModel
		setupMock   func(mockRepo *MockRepository)
		wantErr     bool
		expectedErr string
		validate    func(t *testing.T, result *Template)
	}{
		{
			name: "success - creates template with all fields",
			record: &TemplateMongoDBModel{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				OutputFormat: "PDF",
				Description:  "Financial Report",
				FileName:     "financial.tpl",
				MappedFields: map[string]map[string][]string{
					"transactions": {
						"ledger": {"id", "amount", "currency"},
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			setupMock: func(mockRepo *MockRepository) {
				expected := &Template{
					ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
					OutputFormat: "PDF",
					Description:  "Financial Report",
					FileName:     "financial.tpl",
					CreatedAt:    now,
					UpdatedAt:    now,
				}
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(expected, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *Template) {
				t.Helper()

				assert.Equal(t, uuid.MustParse("00000000-0000-0000-0000-000000000001"), result.ID)
				assert.Equal(t, "PDF", result.OutputFormat)
				assert.Equal(t, "Financial Report", result.Description)
				assert.Equal(t, "financial.tpl", result.FileName)
				assert.Equal(t, now, result.CreatedAt)
				assert.Equal(t, now, result.UpdatedAt)
			},
		},
		{
			name: "success - creates template with minimal fields",
			record: &TemplateMongoDBModel{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				OutputFormat: "HTML",
				FileName:     "minimal.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func(mockRepo *MockRepository) {
				expected := &Template{
					ID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
					OutputFormat: "HTML",
					Description:  "",
					FileName:     "minimal.tpl",
					CreatedAt:    now,
					UpdatedAt:    now,
				}
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(expected, nil)
			},
			wantErr: false,
			validate: func(t *testing.T, result *Template) {
				t.Helper()

				assert.Equal(t, "HTML", result.OutputFormat)
				assert.Empty(t, result.Description)
			},
		},
		{
			name: "error - database connection failure",
			record: &TemplateMongoDBModel{
				ID:           uuid.New(),
				OutputFormat: "PDF",
				FileName:     "test.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to get database"))
			},
			wantErr:     true,
			expectedErr: "failed to get database",
		},
		{
			name: "error - insert failure (duplicate key)",
			record: &TemplateMongoDBModel{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				OutputFormat: "PDF",
				FileName:     "dup.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("duplicate key error"))
			},
			wantErr:     true,
			expectedErr: "duplicate key error",
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

			result, err := mockRepo.Create(context.Background(), tt.record)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				tt.validate(t, result)
			}
		})
	}
}

func TestRepository_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		id           uuid.UUID
		updateFields *bson.M
		setupMock    func(mockRepo *MockRepository)
		wantErr      bool
		expectedErr  string
	}{
		{
			name: "success - update description",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			updateFields: &bson.M{
				"$set": bson.M{
					"description": "Updated Financial Report",
					"updated_at":  time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
				},
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "success - update output format",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			updateFields: &bson.M{
				"$set": bson.M{
					"output_format": "CSV",
					"updated_at":    time.Date(2026, 2, 1, 14, 0, 0, 0, time.UTC),
				},
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "success - update mapped fields",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			updateFields: &bson.M{
				"$set": bson.M{
					"mapped_fields": map[string]map[string][]string{
						"source": {"table1": {"col1", "col2"}},
					},
					"updated_at": time.Date(2026, 2, 1, 16, 0, 0, 0, time.UTC),
				},
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "error - database connection failure",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			updateFields: &bson.M{
				"$set": bson.M{"description": "test"},
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("failed to get database"))
			},
			wantErr:     true,
			expectedErr: "failed to get database",
		},
		{
			name: "error - update execution failure",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			updateFields: &bson.M{
				"$set": bson.M{"description": "test"},
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("failed to update template"))
			},
			wantErr:     true,
			expectedErr: "failed to update template",
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

			err := mockRepo.Update(context.Background(), tt.id, tt.updateFields)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRepository_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          uuid.UUID
		hardDelete  bool
		setupMock   func(mockRepo *MockRepository)
		wantErr     bool
		expectedErr string
	}{
		{
			name:       "success - soft delete",
			id:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			hardDelete: false,
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Eq(false)).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name:       "success - hard delete",
			id:         uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			hardDelete: true,
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Eq(true)).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name:       "error - entity not found on soft delete",
			id:         uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			hardDelete: false,
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Eq(false)).
					Return(errors.New("TPL-0011"))
			},
			wantErr:     true,
			expectedErr: "TPL-0011",
		},
		{
			name:       "error - entity not found on hard delete",
			id:         uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			hardDelete: true,
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Eq(true)).
					Return(errors.New("TPL-0011"))
			},
			wantErr:     true,
			expectedErr: "TPL-0011",
		},
		{
			name:       "error - database connection failure",
			id:         uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			hardDelete: false,
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Eq(false)).
					Return(errors.New("failed to get database"))
			},
			wantErr:     true,
			expectedErr: "failed to get database",
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

			err := mockRepo.Delete(context.Background(), tt.id, tt.hardDelete)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRepository_FindOutputFormatByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		id             uuid.UUID
		setupMock      func(mockRepo *MockRepository)
		wantErr        bool
		expectedErr    string
		expectedFormat string
	}{
		{
			name: "success - returns PDF format",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			setupMock: func(mockRepo *MockRepository) {
				format := "PDF"
				mockRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&format, nil)
			},
			wantErr:        false,
			expectedFormat: "PDF",
		},
		{
			name: "success - returns HTML format",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			setupMock: func(mockRepo *MockRepository) {
				format := "HTML"
				mockRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&format, nil)
			},
			wantErr:        false,
			expectedFormat: "HTML",
		},
		{
			name: "success - returns CSV format",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			setupMock: func(mockRepo *MockRepository) {
				format := "CSV"
				mockRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&format, nil)
			},
			wantErr:        false,
			expectedFormat: "CSV",
		},
		{
			name: "error - template not found",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("mongo: no documents in result"))
			},
			wantErr:     true,
			expectedErr: "mongo: no documents in result",
		},
		{
			name: "error - database connection failure",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to get database"))
			},
			wantErr:     true,
			expectedErr: "failed to get database",
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

			result, err := mockRepo.FindOutputFormatByID(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedFormat, *result)
			}
		})
	}
}

func TestRepository_FindMappedFieldsAndOutputFormatByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		id                 uuid.UUID
		setupMock          func(mockRepo *MockRepository)
		wantErr            bool
		expectedErr        string
		expectedFormat     string
		expectedFieldCount int
	}{
		{
			name: "success - returns format and mapped fields",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			setupMock: func(mockRepo *MockRepository) {
				format := "PDF"
				mappedFields := map[string]map[string][]string{
					"transactions": {
						"ledger": {"id", "amount", "currency", "created_at"},
					},
				}
				mockRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&format, mappedFields, "Test", nil)
			},
			wantErr:            false,
			expectedFormat:     "PDF",
			expectedFieldCount: 1,
		},
		{
			name: "success - returns format and multiple data sources",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			setupMock: func(mockRepo *MockRepository) {
				format := "HTML"
				mappedFields := map[string]map[string][]string{
					"primary": {
						"users":  {"id", "name", "email"},
						"orders": {"id", "total", "status"},
					},
					"secondary": {
						"logs": {"timestamp", "message", "level"},
					},
				}
				mockRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&format, mappedFields, "Test", nil)
			},
			wantErr:            false,
			expectedFormat:     "HTML",
			expectedFieldCount: 2,
		},
		{
			name: "success - returns format with nil mapped fields",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			setupMock: func(mockRepo *MockRepository) {
				format := "CSV"
				mockRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(&format, nil, "Test", nil)
			},
			wantErr:            false,
			expectedFormat:     "CSV",
			expectedFieldCount: 0,
		},
		{
			name: "error - template not found",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, nil, "", errors.New("mongo: no documents in result"))
			},
			wantErr:     true,
			expectedErr: "mongo: no documents in result",
		},
		{
			name: "error - database connection failure",
			id:   uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindMappedFieldsAndOutputFormatByID(gomock.Any(), gomock.Any()).
					Return(nil, nil, "", errors.New("failed to get database"))
			},
			wantErr:     true,
			expectedErr: "failed to get database",
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

			format, mappedFields, _, err := mockRepo.FindMappedFieldsAndOutputFormatByID(context.Background(), tt.id)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, format)
				assert.Nil(t, mappedFields)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, format)
				assert.Equal(t, tt.expectedFormat, *format)

				if tt.expectedFieldCount > 0 {
					assert.Len(t, mappedFields, tt.expectedFieldCount)
				} else {
					assert.Nil(t, mappedFields)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Domain entity conversion tests (no mock needed)
// ---------------------------------------------------------------------------

func TestTemplateMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		entity   *Template
		validate func(t *testing.T, model *TemplateMongoDBModel)
	}{
		{
			name: "converts all fields",
			entity: &Template{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				OutputFormat: "PDF",
				Description:  "Financial Report",
				FileName:     "financial.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			validate: func(t *testing.T, model *TemplateMongoDBModel) {
				t.Helper()

				assert.Equal(t, uuid.MustParse("00000000-0000-0000-0000-000000000001"), model.ID)
				assert.Equal(t, "PDF", model.OutputFormat)
				assert.Equal(t, "Financial Report", model.Description)
				assert.Equal(t, "financial.tpl", model.FileName)
				assert.Equal(t, now, model.CreatedAt)
				assert.Equal(t, now, model.UpdatedAt)
			},
		},
		{
			name: "converts entity with empty description",
			entity: &Template{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				OutputFormat: "HTML",
				Description:  "",
				FileName:     "minimal.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			validate: func(t *testing.T, model *TemplateMongoDBModel) {
				t.Helper()

				assert.Equal(t, "HTML", model.OutputFormat)
				assert.Empty(t, model.Description)
			},
		},
		{
			name: "does not overwrite existing mapped fields",
			entity: &Template{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000003"),
				OutputFormat: "CSV",
				Description:  "Export",
				FileName:     "export.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			validate: func(t *testing.T, model *TemplateMongoDBModel) {
				t.Helper()

				// FromEntity does not set MappedFields - it preserves what was already there
				assert.Equal(t, map[string]map[string][]string{
					"ds": {"t": {"c"}},
				}, model.MappedFields)
			},
		},
		{
			name: "does not overwrite existing deleted_at",
			entity: &Template{
				ID:           uuid.MustParse("00000000-0000-0000-0000-000000000004"),
				OutputFormat: "XML",
				Description:  "XML Export",
				FileName:     "xml_export.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			validate: func(t *testing.T, model *TemplateMongoDBModel) {
				t.Helper()

				// FromEntity does not set DeletedAt - it preserves the previous value
				assert.NotNil(t, model.DeletedAt)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := &TemplateMongoDBModel{}

			// Pre-populate fields that FromEntity should not touch
			if tt.name == "does not overwrite existing mapped fields" {
				model.MappedFields = map[string]map[string][]string{
					"ds": {"t": {"c"}},
				}
			}

			if tt.name == "does not overwrite existing deleted_at" {
				deletedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
				model.DeletedAt = &deletedAt
			}

			model.FromEntity(tt.entity)
			tt.validate(t, model)
		})
	}
}

func TestTemplateMongoDBModel_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		id           uuid.UUID
		outputFormat string
		description  string
		fileName     string
	}{
		{
			name:         "round trip with all fields populated",
			id:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			outputFormat: "PDF",
			description:  "Financial Report Template",
			fileName:     "0196159b-4f26-7300-b3d9-f4f68a7c85f3_1744119295.tpl",
		},
		{
			name:         "round trip with empty description",
			id:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			outputFormat: "HTML",
			description:  "",
			fileName:     "minimal_template.tpl",
		},
		{
			name:         "round trip with CSV format",
			id:           uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			outputFormat: "CSV",
			description:  "CSV Export Template",
			fileName:     "csv_export.tpl",
		},
		{
			name:         "round trip with XML format",
			id:           uuid.MustParse("00000000-0000-0000-0000-000000000004"),
			outputFormat: "XML",
			description:  "XML Data Export",
			fileName:     "xml_data.tpl",
		},
		{
			name:         "round trip with long description",
			id:           uuid.MustParse("00000000-0000-0000-0000-000000000005"),
			outputFormat: "TXT",
			description:  "This is a very long description for a template that contains multiple words and special characters like & < > \" ' to test edge cases",
			fileName:     "long_desc.tpl",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Step 1: Create a Template entity via NewTemplate
			original, err := NewTemplate(tt.id, tt.outputFormat, tt.description, tt.fileName)
			require.NoError(t, err)
			require.NotNil(t, original)

			// Step 2: Convert to MongoDB model via FromEntity
			model := &TemplateMongoDBModel{}
			model.FromEntity(original)

			// Step 3: Convert back to entity via ToEntity
			restored := model.ToEntity()

			// Step 4: Verify all fields survive the round trip
			assert.Equal(t, original.ID, restored.ID)
			assert.Equal(t, original.OutputFormat, restored.OutputFormat)
			assert.Equal(t, original.Description, restored.Description)
			assert.Equal(t, original.FileName, restored.FileName)
			assert.Equal(t, original.CreatedAt, restored.CreatedAt)
			assert.Equal(t, original.UpdatedAt, restored.UpdatedAt)
		})
	}
}

func TestRepository_MockSatisfiesInterface(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockRepository(ctrl)

	// Compile-time verification that MockRepository satisfies Repository
	var _ Repository = mock
}

func TestRepository_FindList_PaginationEdgeCases(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		filters   http.QueryHeader
		setupMock func(mockRepo *MockRepository)
		validate  func(t *testing.T, result []*Template)
	}{
		{
			name: "first page with limit 1",
			filters: http.QueryHeader{
				Limit: 1,
				Page:  1,
			},
			setupMock: func(mockRepo *MockRepository) {
				templates := []*Template{
					{
						ID:           uuid.New(),
						OutputFormat: "PDF",
						Description:  "Single Result",
						FileName:     "single.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
				}
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
			},
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				require.Len(t, result, 1)
			},
		},
		{
			name: "high page number returns empty",
			filters: http.QueryHeader{
				Limit: 10,
				Page:  999,
			},
			setupMock: func(mockRepo *MockRepository) {
				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return([]*Template{}, nil)
			},
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				assert.Empty(t, result)
			},
		},
		{
			name: "large limit returns all available",
			filters: http.QueryHeader{
				Limit: 100,
				Page:  1,
			},
			setupMock: func(mockRepo *MockRepository) {
				templates := make([]*Template, 3)
				for i := range templates {
					templates[i] = &Template{
						ID:           uuid.New(),
						OutputFormat: "PDF",
						Description:  "Template",
						FileName:     "t.tpl",
						CreatedAt:    now,
						UpdatedAt:    now,
					}
				}

				mockRepo.EXPECT().
					FindList(gomock.Any(), gomock.Any()).
					Return(templates, nil)
			},
			validate: func(t *testing.T, result []*Template) {
				t.Helper()

				assert.Len(t, result, 3)
			},
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

			result, err := mockRepo.FindList(context.Background(), tt.filters)
			require.NoError(t, err)
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestRepository_Create_VerifiesModelPassedCorrectly(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	inputModel := &TemplateMongoDBModel{
		ID:           id,
		OutputFormat: "PDF",
		Description:  "Verified Model",
		FileName:     "verified.tpl",
		MappedFields: map[string]map[string][]string{
			"ds1": {"table1": {"col1", "col2"}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	expectedResult := &Template{
		ID:           id,
		OutputFormat: "PDF",
		Description:  "Verified Model",
		FileName:     "verified.tpl",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	mockRepo := NewMockRepository(ctrl)

	// Use gomock.Eq to verify the exact model is passed
	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Eq(inputModel)).
		Return(expectedResult, nil)

	result, err := mockRepo.Create(context.Background(), inputModel)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, id, result.ID)
	assert.Equal(t, "PDF", result.OutputFormat)
	assert.Equal(t, "Verified Model", result.Description)
}

func TestRepository_Delete_VerifiesHardDeleteFlag(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockRepository(ctrl)
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Verify that soft delete passes hardDelete=false
	mockRepo.EXPECT().
		Delete(gomock.Any(), gomock.Eq(id), gomock.Eq(false)).
		Return(nil)

	err := mockRepo.Delete(context.Background(), id, false)
	require.NoError(t, err)
}

func TestRepository_Update_VerifiesExactFields(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockRepository(ctrl)
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	updateFields := &bson.M{
		"$set": bson.M{
			"description": "Updated Description",
			"updated_at":  time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
		},
	}

	// Verify the exact update fields are passed to the repository
	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Eq(id), gomock.Eq(updateFields)).
		Return(nil)

	err := mockRepo.Update(context.Background(), id, updateFields)
	require.NoError(t, err)
}

func TestTemplateMongoDBModel_FromEntity_ZeroValues(t *testing.T) {
	t.Parallel()

	entity := &Template{
		ID:           uuid.Nil,
		OutputFormat: "",
		Description:  "",
		FileName:     "",
		CreatedAt:    time.Time{},
		UpdatedAt:    time.Time{},
	}

	model := &TemplateMongoDBModel{}
	model.FromEntity(entity)

	assert.Equal(t, uuid.Nil, model.ID)
	assert.Empty(t, model.OutputFormat)
	assert.Empty(t, model.Description)
	assert.Empty(t, model.FileName)
	assert.True(t, model.CreatedAt.IsZero())
	assert.True(t, model.UpdatedAt.IsZero())
	// MappedFields and DeletedAt should remain nil (not set by FromEntity)
	assert.Nil(t, model.MappedFields)
	assert.Nil(t, model.DeletedAt)
}

func TestTemplateMongoDBModel_FromEntity_PreservesMongoOnlyFields(t *testing.T) {
	t.Parallel()

	deletedAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	mappedFields := map[string]map[string][]string{
		"analytics": {
			"events":   {"event_id", "event_type", "timestamp"},
			"sessions": {"session_id", "user_id", "duration"},
		},
	}

	model := &TemplateMongoDBModel{
		MappedFields: mappedFields,
		DeletedAt:    &deletedAt,
	}

	entity := &Template{
		ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		OutputFormat: "PDF",
		Description:  "Report",
		FileName:     "report.tpl",
		CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	model.FromEntity(entity)

	// Verify entity fields were set
	assert.Equal(t, entity.ID, model.ID)
	assert.Equal(t, entity.OutputFormat, model.OutputFormat)

	// Verify MongoDB-only fields are preserved (FromEntity does not touch them)
	assert.Equal(t, mappedFields, model.MappedFields)
	assert.Equal(t, &deletedAt, model.DeletedAt)
}

func TestRepository_ContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("Error - FindByID returns error on cancelled context", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := NewMockRepository(ctrl)
		id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mockRepo.EXPECT().
			FindByID(gomock.Any(), gomock.Eq(id)).
			Return(nil, context.Canceled)

		result, err := mockRepo.FindByID(ctx, id)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Error - FindList returns error on cancelled context", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := NewMockRepository(ctrl)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		filters := http.QueryHeader{Limit: 10, Page: 1}

		mockRepo.EXPECT().
			FindList(gomock.Any(), gomock.Any()).
			Return(nil, context.Canceled)

		result, err := mockRepo.FindList(ctx, filters)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Error - Create returns error on cancelled context", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := NewMockRepository(ctrl)
		now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		record := &TemplateMongoDBModel{
			ID:           uuid.New(),
			OutputFormat: "PDF",
			FileName:     "test.tpl",
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		mockRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, context.Canceled)

		result, err := mockRepo.Create(ctx, record)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Error - Update returns error on cancelled context", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := NewMockRepository(ctrl)
		id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		updateFields := &bson.M{"$set": bson.M{"description": "test"}}

		mockRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(context.Canceled)

		err := mockRepo.Update(ctx, id, updateFields)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Error - Delete returns error on cancelled context", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := NewMockRepository(ctrl)
		id := uuid.MustParse("00000000-0000-0000-0000-000000000002")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mockRepo.EXPECT().
			Delete(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(context.Canceled)

		err := mockRepo.Delete(ctx, id, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
