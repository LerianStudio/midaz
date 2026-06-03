// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template

import (
	"fmt"
	"time"

	"github.com/LerianStudio/reporter/pkg/constant"

	"github.com/google/uuid"
)

// Template represents the entity model for a template.
// Public fields are required for JSON serialization (json tags) and Swagger documentation.
// This is a documented deviation from Ring's private-field pattern; use NewTemplate() for programmatic creation.
type Template struct {
	ID           uuid.UUID `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	OutputFormat string    `json:"outputFormat" example:"HTML"`
	Description  string    `json:"description" example:"Template Financeiro"`
	FileName     string    `json:"fileName" example:"0196159b-4f26-7300-b3d9-f4f68a7c85f3_1744119295.tpl"`
	CreatedAt    time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt    time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
}

// NewTemplate creates a new Template entity with invariant validation.
// This constructor ensures the Template can never exist in an invalid state.
//
// Parameters:
//   - id: The template UUID (must not be uuid.Nil)
//   - outputFormat: The output format (must not be empty)
//   - description: Optional description (can be empty)
//   - fileName: The template file name (must not be empty)
//
// Returns:
//   - *Template: A validated Template entity
//   - error: Wrapped ErrMissingRequiredFields if any invariant is violated
func NewTemplate(id uuid.UUID, outputFormat, description, fileName string) (*Template, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("template id must not be nil: %w", constant.ErrMissingRequiredFields)
	}

	if outputFormat == "" {
		return nil, fmt.Errorf("template outputFormat must not be empty: %w", constant.ErrMissingRequiredFields)
	}

	if fileName == "" {
		return nil, fmt.Errorf("template fileName must not be empty: %w", constant.ErrMissingRequiredFields)
	}

	now := time.Now()

	return &Template{
		ID:           id,
		OutputFormat: outputFormat,
		Description:  description,
		FileName:     fileName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// ReconstructTemplate creates a Template from persisted data without validation.
// Used only for database hydration where data integrity is already ensured.
func ReconstructTemplate(id uuid.UUID, outputFormat, description, fileName string, createdAt, updatedAt time.Time) *Template {
	return &Template{
		ID:           id,
		OutputFormat: outputFormat,
		Description:  description,
		FileName:     fileName,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}

// TemplateMongoDBModel represents the MongoDB model for a template
type TemplateMongoDBModel struct {
	ID           uuid.UUID                      `bson:"_id"`
	OutputFormat string                         `bson:"output_format"`
	Description  string                         `bson:"description"`
	FileName     string                         `bson:"filename"`
	MappedFields map[string]map[string][]string `bson:"mapped_fields"`
	CreatedAt    time.Time                      `bson:"created_at"`
	UpdatedAt    time.Time                      `bson:"updated_at"`
	DeletedAt    *time.Time                     `bson:"deleted_at"`
}

// ToEntity converts TemplateMongoDBModel to Template using ReconstructTemplate.
func (tm *TemplateMongoDBModel) ToEntity() *Template {
	return ReconstructTemplate(tm.ID, tm.OutputFormat, tm.Description, tm.FileName, tm.CreatedAt, tm.UpdatedAt)
}

// FromEntity populates TemplateMongoDBModel fields from a Template entity.
// MappedFields and DeletedAt are not set by this method because they are
// MongoDB-only concerns not present on the domain entity.
func (tm *TemplateMongoDBModel) FromEntity(t *Template) {
	tm.ID = t.ID
	tm.OutputFormat = t.OutputFormat
	tm.Description = t.Description
	tm.FileName = t.FileName
	tm.CreatedAt = t.CreatedAt
	tm.UpdatedAt = t.UpdatedAt
}

// FromTemplateEntity creates a new TemplateMongoDBModel from a Template domain entity
// and the MongoDB-only fields (mappedFields, deletedAt) that live outside the domain.
// This is the preferred way to build a complete model for persistence.
func FromTemplateEntity(t *Template, mappedFields map[string]map[string][]string) *TemplateMongoDBModel {
	return &TemplateMongoDBModel{
		ID:           t.ID,
		OutputFormat: t.OutputFormat,
		Description:  t.Description,
		FileName:     t.FileName,
		MappedFields: mappedFields,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}
