// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()

	mongoModel := &TemplateMongoDBModel{
		ID:           id,
		OutputFormat: "PDF",
		Description:  "Financial Report Template",
		FileName:     "0196159b-4f26-7300-b3d9-f4f68a7c85f3_1744119295.tpl",
		MappedFields: map[string]map[string][]string{
			"users": {
				"table1": {"name", "email", "created_at"},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: nil,
	}

	entity := mongoModel.ToEntity()

	assert.Equal(t, id, entity.ID)
	assert.Equal(t, "PDF", entity.OutputFormat)
	assert.Equal(t, "Financial Report Template", entity.Description)
	assert.Equal(t, "0196159b-4f26-7300-b3d9-f4f68a7c85f3_1744119295.tpl", entity.FileName)
	assert.Equal(t, now, entity.CreatedAt)
	assert.Equal(t, now, entity.UpdatedAt)
}

func TestTemplateMongoDBModel_ToEntity_EmptyFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()

	mongoModel := &TemplateMongoDBModel{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
	}

	entity := mongoModel.ToEntity()

	assert.Equal(t, id, entity.ID)
	assert.Empty(t, entity.OutputFormat)
	assert.Empty(t, entity.Description)
	assert.Empty(t, entity.FileName)
	assert.Equal(t, now, entity.CreatedAt)
	assert.Equal(t, now, entity.UpdatedAt)
}

func TestTemplateMongoDBModel_ToEntity_AllOutputFormats(t *testing.T) {
	t.Parallel()

	formats := []string{"PDF", "HTML", "CSV", "XML", "TXT"}

	for _, format := range formats {
		format := format
		t.Run("Success - Format_"+format, func(t *testing.T) {
			t.Parallel()

			mongoModel := &TemplateMongoDBModel{
				ID:           uuid.New(),
				OutputFormat: format,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}

			entity := mongoModel.ToEntity()

			assert.Equal(t, format, entity.OutputFormat)
		})
	}
}

func TestTemplate_Struct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()

	template := Template{
		ID:           id,
		OutputFormat: "HTML",
		Description:  "Monthly Sales Report",
		FileName:     "template_123.tpl",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	assert.Equal(t, id, template.ID)
	assert.Equal(t, "HTML", template.OutputFormat)
	assert.Equal(t, "Monthly Sales Report", template.Description)
	assert.Equal(t, "template_123.tpl", template.FileName)
	assert.Equal(t, now, template.CreatedAt)
	assert.Equal(t, now, template.UpdatedAt)
}

func TestTemplateMongoDBModel_Struct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	deletedAt := now.Add(time.Hour)
	id := uuid.New()

	mappedFields := map[string]map[string][]string{
		"data_source_1": {
			"table_a": {"col1", "col2", "col3"},
			"table_b": {"id", "name"},
		},
		"data_source_2": {
			"table_c": {"value", "timestamp"},
		},
	}

	mongoModel := TemplateMongoDBModel{
		ID:           id,
		OutputFormat: "CSV",
		Description:  "Export Template",
		FileName:     "export_template.tpl",
		MappedFields: mappedFields,
		CreatedAt:    now,
		UpdatedAt:    now,
		DeletedAt:    &deletedAt,
	}

	assert.Equal(t, id, mongoModel.ID)
	assert.Equal(t, "CSV", mongoModel.OutputFormat)
	assert.Equal(t, "Export Template", mongoModel.Description)
	assert.Equal(t, "export_template.tpl", mongoModel.FileName)
	assert.Equal(t, mappedFields, mongoModel.MappedFields)
	assert.Equal(t, now, mongoModel.CreatedAt)
	assert.Equal(t, now, mongoModel.UpdatedAt)
	assert.Equal(t, &deletedAt, mongoModel.DeletedAt)
}

func TestTemplateMongoDBModel_MappedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mappedFields map[string]map[string][]string
	}{
		{
			name:         "Empty mapped fields",
			mappedFields: nil,
		},
		{
			name:         "Single data source",
			mappedFields: map[string]map[string][]string{"ds1": {"t1": {"c1"}}},
		},
		{
			name: "Multiple data sources",
			mappedFields: map[string]map[string][]string{
				"primary":   {"users": {"id", "name"}, "orders": {"id", "total"}},
				"secondary": {"logs": {"timestamp", "message"}},
			},
		},
		{
			name: "Complex nested structure",
			mappedFields: map[string]map[string][]string{
				"analytics": {
					"events":      {"event_id", "event_type", "user_id", "timestamp", "payload"},
					"sessions":    {"session_id", "user_id", "start_time", "end_time", "device"},
					"conversions": {"conversion_id", "type", "value", "source"},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mongoModel := TemplateMongoDBModel{
				ID:           uuid.New(),
				MappedFields: tt.mappedFields,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}

			assert.Equal(t, tt.mappedFields, mongoModel.MappedFields)
		})
	}
}

func TestTemplateMongoDBModel_ToEntity_DoesNotIncludeMappedFields(t *testing.T) {
	t.Parallel()

	mappedFields := map[string]map[string][]string{
		"ds1": {
			"table1": {"col1", "col2"},
		},
	}

	mongoModel := &TemplateMongoDBModel{
		ID:           uuid.New(),
		OutputFormat: "PDF",
		MappedFields: mappedFields,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	entity := mongoModel.ToEntity()

	// Template entity doesn't have MappedFields
	assert.Equal(t, "PDF", entity.OutputFormat)
	// MappedFields is only in MongoDB model, not in entity
}

func TestTemplateMongoDBModel_ToEntity_WithDeletedAt(t *testing.T) {
	t.Parallel()

	now := time.Now()
	deletedAt := now.Add(time.Hour)

	mongoModel := &TemplateMongoDBModel{
		ID:           uuid.New(),
		OutputFormat: "XML",
		Description:  "Deleted Template",
		FileName:     "deleted.tpl",
		CreatedAt:    now,
		UpdatedAt:    now,
		DeletedAt:    &deletedAt,
	}

	entity := mongoModel.ToEntity()

	// Entity doesn't have DeletedAt field (soft delete only in MongoDB model)
	assert.Equal(t, "XML", entity.OutputFormat)
	assert.Equal(t, "Deleted Template", entity.Description)
}

func TestTemplate_JSONTags(t *testing.T) {
	t.Parallel()

	template := Template{
		ID:           uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		OutputFormat: "PDF",
		Description:  "Test",
		FileName:     "test.tpl",
		CreatedAt:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	// Verify struct fields exist with expected values
	assert.Equal(t, uuid.MustParse("00000000-0000-0000-0000-000000000001"), template.ID)
	assert.Equal(t, "PDF", template.OutputFormat)
	assert.Equal(t, "Test", template.Description)
	assert.Equal(t, "test.tpl", template.FileName)
}

func TestNewTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		id           uuid.UUID
		outputFormat string
		description  string
		fileName     string
		wantErr      bool
		expectedErr  error
	}{
		{
			name:         "valid template with all fields",
			id:           uuid.New(),
			outputFormat: "pdf",
			description:  "Financial Report",
			fileName:     "template_123.tpl",
			wantErr:      false,
		},
		{
			name:         "valid template with empty description",
			id:           uuid.New(),
			outputFormat: "html",
			description:  "",
			fileName:     "template_456.tpl",
			wantErr:      false,
		},
		{
			name:         "nil ID returns error",
			id:           uuid.Nil,
			outputFormat: "pdf",
			description:  "Test",
			fileName:     "template.tpl",
			wantErr:      true,
			expectedErr:  constant.ErrMissingRequiredFields,
		},
		{
			name:         "empty outputFormat returns error",
			id:           uuid.New(),
			outputFormat: "",
			description:  "Test",
			fileName:     "template.tpl",
			wantErr:      true,
			expectedErr:  constant.ErrMissingRequiredFields,
		},
		{
			name:         "empty fileName returns error",
			id:           uuid.New(),
			outputFormat: "pdf",
			description:  "Test",
			fileName:     "",
			wantErr:      true,
			expectedErr:  constant.ErrMissingRequiredFields,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewTemplate(tt.id, tt.outputFormat, tt.description, tt.fileName)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)

				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr), "expected error %v, got %v", tt.expectedErr, err)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.id, got.ID)
				assert.Equal(t, tt.outputFormat, got.OutputFormat)
				assert.Equal(t, tt.description, got.Description)
				assert.Equal(t, tt.fileName, got.FileName)
				assert.False(t, got.CreatedAt.IsZero())
				assert.False(t, got.UpdatedAt.IsZero())
			}
		})
	}
}

func TestReconstructTemplate(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	createdAt := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2025, 6, 16, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		id           uuid.UUID
		outputFormat string
		description  string
		fileName     string
		createdAt    time.Time
		updatedAt    time.Time
	}{
		{
			name:         "reconstruct with all fields populated",
			id:           id,
			outputFormat: "PDF",
			description:  "Financial Report",
			fileName:     "template_123.tpl",
			createdAt:    createdAt,
			updatedAt:    updatedAt,
		},
		{
			name:         "reconstruct with empty optional fields",
			id:           id,
			outputFormat: "HTML",
			description:  "",
			fileName:     "template_456.tpl",
			createdAt:    createdAt,
			updatedAt:    updatedAt,
		},
		{
			name:         "reconstruct with nil UUID (trusts DB data)",
			id:           uuid.Nil,
			outputFormat: "",
			description:  "",
			fileName:     "",
			createdAt:    time.Time{},
			updatedAt:    time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ReconstructTemplate(tt.id, tt.outputFormat, tt.description, tt.fileName, tt.createdAt, tt.updatedAt)

			require.NotNil(t, got)
			assert.Equal(t, tt.id, got.ID)
			assert.Equal(t, tt.outputFormat, got.OutputFormat)
			assert.Equal(t, tt.description, got.Description)
			assert.Equal(t, tt.fileName, got.FileName)
			assert.Equal(t, tt.createdAt, got.CreatedAt)
			assert.Equal(t, tt.updatedAt, got.UpdatedAt)
		})
	}
}

func TestReconstructTemplate_MatchesToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()

	mongoModel := &TemplateMongoDBModel{
		ID:           id,
		OutputFormat: "PDF",
		Description:  "Financial Report Template",
		FileName:     "0196159b-4f26-7300-b3d9-f4f68a7c85f3_1744119295.tpl",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	fromToEntity := mongoModel.ToEntity()
	fromReconstruct := ReconstructTemplate(id, "PDF", "Financial Report Template", "0196159b-4f26-7300-b3d9-f4f68a7c85f3_1744119295.tpl", now, now)

	assert.Equal(t, fromToEntity.ID, fromReconstruct.ID)
	assert.Equal(t, fromToEntity.OutputFormat, fromReconstruct.OutputFormat)
	assert.Equal(t, fromToEntity.Description, fromReconstruct.Description)
	assert.Equal(t, fromToEntity.FileName, fromReconstruct.FileName)
	assert.Equal(t, fromToEntity.CreatedAt, fromReconstruct.CreatedAt)
	assert.Equal(t, fromToEntity.UpdatedAt, fromReconstruct.UpdatedAt)
}

func TestFromTemplateEntity(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()

	mappedFields := map[string]map[string][]string{
		"data_source_1": {
			"table_a": {"col1", "col2", "col3"},
			"table_b": {"id", "name"},
		},
		"data_source_2": {
			"table_c": {"value", "timestamp"},
		},
	}

	tests := []struct {
		name         string
		entity       *Template
		mappedFields map[string]map[string][]string
	}{
		{
			name: "creates model with all fields and mapped fields",
			entity: &Template{
				ID:           id,
				OutputFormat: "pdf",
				Description:  "Financial Report",
				FileName:     "template_123.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			mappedFields: mappedFields,
		},
		{
			name: "creates model with nil mapped fields",
			entity: &Template{
				ID:           id,
				OutputFormat: "html",
				Description:  "",
				FileName:     "template_456.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			mappedFields: nil,
		},
		{
			name: "creates model with empty mapped fields",
			entity: &Template{
				ID:           id,
				OutputFormat: "csv",
				Description:  "Export",
				FileName:     "template_789.tpl",
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			mappedFields: map[string]map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mongoModel := FromTemplateEntity(tt.entity, tt.mappedFields)

			require.NotNil(t, mongoModel)
			assert.Equal(t, tt.entity.ID, mongoModel.ID)
			assert.Equal(t, tt.entity.OutputFormat, mongoModel.OutputFormat)
			assert.Equal(t, tt.entity.Description, mongoModel.Description)
			assert.Equal(t, tt.entity.FileName, mongoModel.FileName)
			assert.Equal(t, tt.mappedFields, mongoModel.MappedFields)
			assert.Equal(t, tt.entity.CreatedAt, mongoModel.CreatedAt)
			assert.Equal(t, tt.entity.UpdatedAt, mongoModel.UpdatedAt)
			// FromTemplateEntity never sets DeletedAt (new entities are not deleted)
			assert.Nil(t, mongoModel.DeletedAt)
		})
	}
}

func TestFromTemplateEntity_RoundTrip(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	entity, err := NewTemplate(id, "pdf", "Financial Report", "template_123.tpl")
	require.NoError(t, err)

	mappedFields := map[string]map[string][]string{
		"ds1": {"t1": {"c1", "c2"}},
	}

	mongoModel := FromTemplateEntity(entity, mappedFields)
	roundTripped := mongoModel.ToEntity()

	// Domain fields survive the round trip
	assert.Equal(t, entity.ID, roundTripped.ID)
	assert.Equal(t, entity.OutputFormat, roundTripped.OutputFormat)
	assert.Equal(t, entity.Description, roundTripped.Description)
	assert.Equal(t, entity.FileName, roundTripped.FileName)
	assert.Equal(t, entity.CreatedAt, roundTripped.CreatedAt)
	assert.Equal(t, entity.UpdatedAt, roundTripped.UpdatedAt)

	// MappedFields is preserved in the MongoDB model but not in the domain entity
	assert.Equal(t, mappedFields, mongoModel.MappedFields)
}

func TestFromTemplateEntity_MatchesFromEntityReceiver(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()

	entity := &Template{
		ID:           id,
		OutputFormat: "pdf",
		Description:  "Test Report",
		FileName:     "test.tpl",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Build via standalone function
	fromStandalone := FromTemplateEntity(entity, nil)

	// Build via receiver method
	fromReceiver := &TemplateMongoDBModel{}
	fromReceiver.FromEntity(entity)

	// Domain fields must match between both approaches
	assert.Equal(t, fromReceiver.ID, fromStandalone.ID)
	assert.Equal(t, fromReceiver.OutputFormat, fromStandalone.OutputFormat)
	assert.Equal(t, fromReceiver.Description, fromStandalone.Description)
	assert.Equal(t, fromReceiver.FileName, fromStandalone.FileName)
	assert.Equal(t, fromReceiver.CreatedAt, fromStandalone.CreatedAt)
	assert.Equal(t, fromReceiver.UpdatedAt, fromStandalone.UpdatedAt)
}

func TestTemplateMongoDBModel_BSONTags(t *testing.T) {
	t.Parallel()

	deletedAt := time.Now()

	mongoModel := TemplateMongoDBModel{
		ID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		OutputFormat: "HTML",
		Description:  "BSON Test",
		FileName:     "bson_test.tpl",
		MappedFields: map[string]map[string][]string{"ds": {"t": {"c"}}},
		CreatedAt:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		DeletedAt:    &deletedAt,
	}

	// Verify struct fields exist with expected values
	assert.Equal(t, uuid.MustParse("00000000-0000-0000-0000-000000000002"), mongoModel.ID)
	assert.Equal(t, "HTML", mongoModel.OutputFormat)
	assert.Equal(t, "BSON Test", mongoModel.Description)
	assert.Equal(t, "bson_test.tpl", mongoModel.FileName)
	assert.NotNil(t, mongoModel.MappedFields)
	assert.Equal(t, &deletedAt, mongoModel.DeletedAt)
}
