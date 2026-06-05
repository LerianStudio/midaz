// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package extraction

import (
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
)

// ExtractionMappingMongoDBModel represents the MongoDB document model for ExtractionMapping.
type ExtractionMappingMongoDBModel struct {
	JobID        string     `bson:"job_id"`
	ReportID     string     `bson:"report_id"`
	TemplateID   string     `bson:"template_id"`
	TenantID     string     `bson:"tenant_id"`
	OutputFormat string     `bson:"output_format"`
	Status       string     `bson:"status"`
	CreatedAt    time.Time  `bson:"created_at"`
	CompletedAt  *time.Time `bson:"completed_at,omitempty"`
}

// FromEntity converts an ExtractionMapping domain entity into the MongoDB model.
func (m *ExtractionMappingMongoDBModel) FromEntity(e *datasource.ExtractionMapping) {
	m.JobID = e.JobID
	m.ReportID = e.ReportID
	m.TemplateID = e.TemplateID
	m.TenantID = e.TenantID
	m.OutputFormat = e.OutputFormat
	m.Status = e.Status
	m.CreatedAt = e.CreatedAt
	m.CompletedAt = e.CompletedAt
}

// ToEntity converts the MongoDB model into an ExtractionMapping domain entity.
func (m *ExtractionMappingMongoDBModel) ToEntity() *datasource.ExtractionMapping {
	return &datasource.ExtractionMapping{
		JobID:        m.JobID,
		ReportID:     m.ReportID,
		TemplateID:   m.TemplateID,
		TenantID:     m.TenantID,
		OutputFormat: m.OutputFormat,
		Status:       m.Status,
		CreatedAt:    m.CreatedAt,
		CompletedAt:  m.CompletedAt,
	}
}
