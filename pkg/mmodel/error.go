// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// Error represents a standardized API error response format
type Error struct {
	// Error code identifying the specific error condition
	// example: 0147
	// maxLength: 50
	Code string `json:"code" validate:"required" example:"0147" maxLength:"50"`

	// Short, human-readable error title
	// example: Bad Request
	// maxLength: 100
	Title string `json:"title" validate:"required" example:"Bad Request" maxLength:"100"`

	// Detailed error message explaining the issue
	// example: The request contains invalid fields. Please check the field 'name' and try again.
	// maxLength: 500
	Message string `json:"message" validate:"required" example:"The request contains invalid fields. Please check the field 'name' and try again." maxLength:"500"`

	// Optional type of entity associated with the error
	// example: Organization
	// maxLength: 100
	EntityType string `json:"entityType,omitempty" example:"Organization" maxLength:"100"`

	// Optional detailed field validations for client-side handling
	// example: {"name": "Field 'name' is required"}
	Fields map[string]string `json:"fields,omitempty"`
}

// ErrorResponse represents a standardized API error response
type ErrorResponse struct {
	Body Error
}
