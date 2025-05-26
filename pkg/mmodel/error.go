package mmodel

// Error represents a standardized API error response format
//
// swagger:model Error
// @Description Standardized error response format used across all API endpoints for error situations. Provides structured information about errors including codes, messages, and field-specific validation details.
type Error struct {
	// Error code identifying the specific error condition
	// example: ERR_INVALID_INPUT
	// maxLength: 50
	Code string `json:"code" example:"ERR_INVALID_INPUT" maxLength:"50"`

	// Short, human-readable error title
	// example: Bad Request
	// maxLength: 100
	Title string `json:"title" example:"Bad Request" maxLength:"100"`

	// Detailed error message explaining the issue
	// example: The request contains invalid fields. Please check the field 'name' and try again.
	// maxLength: 500
	Message string `json:"message" example:"The request contains invalid fields. Please check the field 'name' and try again." maxLength:"500"`

	// Optional type of entity associated with the error
	// example: Organization
	// maxLength: 100
	EntityType string `json:"entityType,omitempty" example:"Organization" maxLength:"100"`

	// Optional detailed field validations for client-side handling
	// example: {"name": "Field 'name' is required"}
	Fields map[string]string `json:"fields,omitempty" example:"{\"name\": \"Field 'name' is required\"}"`
} // @name Error

// ErrorResponse represents a standardized API error response
//
// swagger:response ErrorResponse
// @Description Standard error response format returned by all API endpoints for error situations.
type ErrorResponse struct {
	// in: body
	Body Error
}