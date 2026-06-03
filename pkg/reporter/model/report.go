// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"

	"github.com/google/uuid"
)

// FilterCondition defines advanced filtering conditions for report generation.
// Supports multiple operators for complex queries including range, equality, and list-based filters.
// Public fields are required for JSON deserialization (json tags) and request validation (validate tags).
// This is a documented deviation from Ring's private-field domain model pattern.
type FilterCondition struct {
	// Equals specifies exact value matches. Multiple values treated as OR conditions.
	// Example: {"eq": ["active", "pending"]} matches records where field equals "active" OR "pending"
	Equals []any `json:"eq,omitempty"`

	// GreaterThan specifies values that must be greater than the provided value.
	// Should contain exactly one value for comparison.
	// Example: {"gt": [100]} matches records where field > 100
	GreaterThan []any `json:"gt,omitempty"`

	// GreaterOrEqual specifies values that must be greater than or equal to the provided value.
	// Should contain exactly one value for comparison.
	// Example: {"gte": ["2025-06-01"]} matches records where field >= "2025-06-01"
	GreaterOrEqual []any `json:"gte,omitempty"`

	// LessThan specifies values that must be less than the provided value.
	// Should contain exactly one value for comparison.
	// Example: {"lt": [1000]} matches records where field < 1000
	LessThan []any `json:"lt,omitempty"`

	// LessOrEqual specifies values that must be less than or equal to the provided value.
	// Should contain exactly one value for comparison.
	// Example: {"lte": ["2025-06-30"]} matches records where field <= "2025-06-30"
	LessOrEqual []any `json:"lte,omitempty"`

	// Between specifies a range condition with exactly two values [min, max].
	// Matches records where min <= field <= max
	// Example: {"between": [100, 1000]} matches records where 100 <= field <= 1000
	Between []any `json:"between,omitempty"`

	// In specifies a list of values where the field must match any one of them.
	// Multiple values treated as OR conditions.
	// Example: {"in": ["active", "pending", "suspended"]} matches any of these statuses
	In []any `json:"in,omitempty"`

	// NotIn specifies a list of values where the field must NOT match any of them.
	// Multiple values treated as AND NOT conditions.
	// Example: {"nin": ["deleted", "archived"]} excludes these statuses
	NotIn []any `json:"nin,omitempty"`
}

// CreateReportInput is a struct designed to encapsulate request create payload data.
// Public fields are required for JSON binding (json tags) and validation (validate tags).
// This is a documented deviation from Ring's private-field pattern; use NewCreateReportInput() for programmatic creation.
//
// swagger:model CreateReportInput
//
//	@Description	CreateReportInput is the input payload to create a report.
type CreateReportInput struct {
	TemplateID string                                           `json:"templateId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	Filters    map[string]map[string]map[string]FilterCondition `json:"filters" validate:"required"`
} //	@name	CreateReportInput

// NewCreateReportInput creates a new CreateReportInput with validation.
// This constructor enforces domain invariants at creation time, ensuring the input
// can never exist in an invalid state.
//
// Parameters:
//   - templateID: The template identifier (must not be empty)
//   - filters: The filter conditions for report generation
//
// Returns:
//   - *CreateReportInput: A validated input object
//   - error: Validation error if invariants are violated
func NewCreateReportInput(templateID string, filters map[string]map[string]map[string]FilterCondition) (*CreateReportInput, error) {
	if templateID == "" {
		return nil, fmt.Errorf("templateID must not be empty")
	}

	if _, err := uuid.Parse(templateID); err != nil {
		return nil, fmt.Errorf("templateID must be a valid UUID: %w", err)
	}

	return &CreateReportInput{
		TemplateID: templateID,
		Filters:    filters,
	}, nil
}

// ReportMessage is a struct designed to encapsulate response payload data.
//
// swagger:model ReportMessage
//
//	@Description	ReportMessage represents a report struct of message sent it in RabbitMQ
type ReportMessage struct {
	TemplateID   uuid.UUID                                        `json:"templateId" example:"00000000-0000-0000-0000-000000000000"`
	ReportID     uuid.UUID                                        `json:"reportId" example:"00000000-0000-0000-0000-000000000000"`
	OutputFormat string                                           `json:"outputFormat" example:"html"`
	Filters      map[string]map[string]map[string]FilterCondition `json:"filters"`
	MappedFields map[string]map[string][]string                   `json:"mappedFields"`
} //	@name	ReportMessage

// NewReportMessage creates a new ReportMessage with validation.
// This constructor enforces domain invariants at creation time, ensuring the message
// can never exist in an invalid state before being published to RabbitMQ.
//
// Parameters:
//   - templateID: The template UUID (must not be nil)
//   - reportID: The report UUID (must not be nil)
//   - outputFormat: The desired output format (must not be empty)
//   - mappedFields: Field mappings for template rendering (can be nil)
//   - filters: Filter conditions for data queries (can be nil)
//
// Returns:
//   - *ReportMessage: A validated message object
//   - error: Validation error if invariants are violated
func NewReportMessage(
	templateID, reportID uuid.UUID,
	outputFormat string,
	mappedFields map[string]map[string][]string,
	filters map[string]map[string]map[string]FilterCondition,
) (*ReportMessage, error) {
	if templateID == uuid.Nil {
		return nil, fmt.Errorf("templateID must not be nil")
	}

	if reportID == uuid.Nil {
		return nil, fmt.Errorf("reportID must not be nil")
	}

	if outputFormat == "" {
		return nil, fmt.Errorf("outputFormat must not be empty")
	}

	return &ReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: outputFormat,
		MappedFields: mappedFields,
		Filters:      filters,
	}, nil
}
