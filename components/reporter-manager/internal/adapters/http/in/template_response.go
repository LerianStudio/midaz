// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/reporter/pkg/datasource"
	"github.com/LerianStudio/reporter/pkg/mongodb/template"
)

// TemplateResponse wraps a Template with optional warnings from schema
// validation. When DataSourceProvider validates mapped fields and a data source
// is unavailable (D7 decision), warnings are collected and returned alongside
// the template instead of failing the request.
//
// swagger:model TemplateResponse
//
//	@Description	TemplateResponse wraps a template with optional validation warnings.
type TemplateResponse struct {
	template.Template
	Warnings []datasource.ValidationWarning `json:"warnings,omitempty"`
} //	@name	TemplateResponse

// newTemplateResponse creates a TemplateResponse from a template and optional warnings.
// If warnings is nil or empty, the Warnings field will be omitted from JSON serialization.
func newTemplateResponse(tmpl *template.Template, warnings []datasource.ValidationWarning) *TemplateResponse {
	if tmpl == nil {
		return nil
	}

	return &TemplateResponse{
		Template: *tmpl,
		Warnings: warnings,
	}
}
