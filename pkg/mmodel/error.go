// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

// Error represents a standardized API error response format
//
// swagger:model Error
// @Description RFC-9457-aligned error body: a stable machine-readable code, a title, a human message, and optional entityType/fields.
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
} // @name Error

// ErrorResponse represents a standardized API error response
//
// swagger:response ErrorResponse
// @Description Standard error response format returned by all API endpoints for error situations.
type ErrorResponse struct {
	// in: body
	Body Error
}
