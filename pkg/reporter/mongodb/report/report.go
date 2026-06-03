// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package report

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/model"

	"github.com/google/uuid"
)

// Report represents the entity model for a report.
// Public fields are required for JSON serialization (json tags) and Swagger documentation.
// This is a documented deviation from Ring's private-field pattern; use NewReport() for programmatic creation.
type Report struct {
	ID                   uuid.UUID                                              `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	TemplateID           uuid.UUID                                              `json:"templateId" example:"00000000-0000-0000-0000-000000000000"`
	TemplateOutputFormat string                                                 `json:"templateOutputFormat,omitempty" example:"xml"`
	TemplateDescription  string                                                 `json:"templateDescription,omitempty" example:"Cadoc"`
	Filters              map[string]map[string]map[string]model.FilterCondition `json:"filters"`
	Status               string                                                 `json:"status" example:"processing"`
	Metadata             map[string]any                                         `json:"metadata"`
	CompletedAt          *time.Time                                             `json:"completedAt"`
	CreatedAt            time.Time                                              `json:"createdAt"`
	UpdatedAt            time.Time                                              `json:"updatedAt"`
	DeletedAt            *time.Time                                             `json:"deletedAt"`
}

// NewReport creates a new Report entity with invariant validation.
// This constructor ensures the Report can never exist in an invalid state.
//
// Parameters:
//   - id: The report UUID (must not be uuid.Nil)
//   - templateID: The template UUID (must not be uuid.Nil)
//   - status: The report status (must not be empty)
//   - filters: Optional filter conditions for report generation (can be nil)
//
// Returns:
//   - *Report: A validated Report entity
//   - error: Wrapped ErrMissingRequiredFields if any invariant is violated
func NewReport(
	id, templateID uuid.UUID,
	status string,
	filters map[string]map[string]map[string]model.FilterCondition,
	templateOutputFormat, templateDescription string,
) (*Report, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("report id must not be nil: %w", constant.ErrMissingRequiredFields)
	}

	if templateID == uuid.Nil {
		return nil, fmt.Errorf("report templateID must not be nil: %w", constant.ErrMissingRequiredFields)
	}

	if status == "" {
		return nil, fmt.Errorf("report status must not be empty: %w", constant.ErrMissingRequiredFields)
	}

	now := time.Now()

	return &Report{
		ID:                   id,
		TemplateID:           templateID,
		TemplateOutputFormat: templateOutputFormat,
		TemplateDescription:  templateDescription,
		Status:               status,
		Filters:              filters,
		CreatedAt:            now,
		UpdatedAt:            now,
	}, nil
}

// ReconstructReport creates a Report from persisted data without validation.
// Used only for database hydration where data integrity is already ensured.
func ReconstructReport(
	id, templateID uuid.UUID,
	status string,
	filters map[string]map[string]map[string]model.FilterCondition,
	metadata map[string]any,
	completedAt *time.Time,
	createdAt, updatedAt time.Time,
	deletedAt *time.Time,
	templateOutputFormat, templateDescription string,
) *Report {
	return &Report{
		ID:                   id,
		TemplateID:           templateID,
		TemplateOutputFormat: templateOutputFormat,
		TemplateDescription:  templateDescription,
		Status:               status,
		Filters:              filters,
		Metadata:             metadata,
		CompletedAt:          completedAt,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		DeletedAt:            deletedAt,
	}
}

// ReportMongoDBModel represents the MongoDB model for a report
type ReportMongoDBModel struct {
	ID                   uuid.UUID                                              `bson:"_id"`
	TemplateID           uuid.UUID                                              `bson:"template_id"`
	TemplateOutputFormat string                                                 `bson:"template_output_format,omitempty"`
	TemplateDescription  string                                                 `bson:"template_description,omitempty"`
	Status               string                                                 `bson:"status"`
	Filters              map[string]map[string]map[string]model.FilterCondition `bson:"filters"`
	Metadata             map[string]any                                         `bson:"metadata"`
	CompletedAt          *time.Time                                             `bson:"completed_at"`
	CreatedAt            time.Time                                              `bson:"created_at"`
	UpdatedAt            time.Time                                              `bson:"updated_at"`
	DeletedAt            *time.Time                                             `bson:"deleted_at"`
}

// ToEntity converts ReportMongoDBModel to Report using ReconstructReport.
func (rm *ReportMongoDBModel) ToEntity(filters map[string]map[string]map[string]model.FilterCondition) *Report {
	return ReconstructReport(rm.ID, rm.TemplateID, rm.Status, filters, nil, rm.CompletedAt, rm.CreatedAt, rm.UpdatedAt, rm.DeletedAt, rm.TemplateOutputFormat, rm.TemplateDescription)
}

// ToEntityFindByID converts ReportMongoDBModel to Report using ReconstructReport.
func (rm *ReportMongoDBModel) ToEntityFindByID() *Report {
	return ReconstructReport(rm.ID, rm.TemplateID, rm.Status, rm.Filters, rm.Metadata, rm.CompletedAt, rm.CreatedAt, rm.UpdatedAt, rm.DeletedAt, rm.TemplateOutputFormat, rm.TemplateDescription)
}

// FromEntity converts Report to ReportMongoDBModel
func (rm *ReportMongoDBModel) FromEntity(r *Report) error {
	dateNow := time.Now()
	rm.ID = r.ID
	rm.TemplateID = r.TemplateID
	rm.TemplateOutputFormat = r.TemplateOutputFormat
	rm.TemplateDescription = r.TemplateDescription
	rm.Metadata = r.Metadata
	rm.Status = r.Status
	rm.Filters = r.Filters
	rm.CompletedAt = r.CompletedAt
	rm.CreatedAt = dateNow
	rm.UpdatedAt = dateNow
	rm.DeletedAt = nil

	return nil
}
