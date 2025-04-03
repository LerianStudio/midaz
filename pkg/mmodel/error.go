package mmodel

// Error represents a standardized API error response format
//
// swagger:model Error
// @Description Standardized error response format used across all API endpoints for error situations
type Error struct {
	// Error code identifying the specific error condition
	// example: ERR_INVALID_INPUT
	Code string `json:"code"`

	// Short, human-readable error title
	// example: Bad Request
	Title string `json:"title"`

	// Detailed error message explaining the issue
	// example: The request contains invalid fields. Please check the field 'name' and try again.
	Message string `json:"message"`

	// Optional type of entity associated with the error
	// example: Organization
	EntityType string `json:"entityType,omitempty"`

	// Optional detailed field validations for client-side handling
	// example: {"name": "Field 'name' is required"}
	Fields map[string]string `json:"fields,omitempty"`
} // @name Error