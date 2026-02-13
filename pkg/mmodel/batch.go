package mmodel

import (
	"encoding/json"
)

// BatchRequest represents a unified batch request containing multiple API requests.
//
// swagger:model BatchRequest
//
//	@Description	Request payload for batch processing multiple API requests in a single HTTP call.
//	@example		{
//	  "requests": [
//	    {
//	      "id": "req-1",
//	      "method": "POST",
//	      "path": "/v1/organizations",
//	      "body": {"legalName": "Acme Corp", "legalDocument": "12345678901234"}
//	    },
//	    {
//	      "id": "req-2",
//	      "method": "GET",
//	      "path": "/v1/organizations"
//	    }
//	  ]
//	}
type BatchRequest struct {
	// Array of request items to process in the batch
	// required: true
	// minItems: 1
	// maxItems: 100
	Requests []BatchRequestItem `json:"requests" validate:"required,min=1,max=100,dive"`
} //	@name	BatchRequest

// BatchRequestItem represents a single request within a batch.
//
// swagger:model BatchRequestItem
//
//	@Description	A single API request to be processed as part of a batch operation.
//	@example		{
//	  "id": "req-1",
//	  "method": "POST",
//	  "path": "/v1/organizations",
//	  "headers": {"X-Custom-Header": "value"},
//	  "body": {"legalName": "Acme Corp"}
//	}
type BatchRequestItem struct {
	// Unique identifier for this request within the batch (used to correlate responses)
	// required: true
	// example: req-1
	// maxLength: 100
	ID string `json:"id" validate:"required,max=100" example:"req-1" maxLength:"100"`

	// HTTP method for this request
	// required: true
	// example: POST
	// enum: GET,POST,PUT,PATCH,DELETE,HEAD
	Method string `json:"method" validate:"required,oneof=GET POST PUT PATCH DELETE HEAD" example:"POST"`

	// API path for this request (relative to the API base)
	// required: true
	// example: /v1/organizations
	// maxLength: 500
	Path string `json:"path" validate:"required,max=500" example:"/v1/organizations" maxLength:"500"`

	// Optional headers to include with this request (security-critical headers like Authorization cannot be overridden)
	// required: false
	Headers map[string]string `json:"headers,omitempty"`

	// Optional request body (for POST, PUT, PATCH methods)
	// required: false
	Body json.RawMessage `json:"body,omitempty"`
} //	@name	BatchRequestItem

// BatchResponse represents the response from a batch operation.
//
// swagger:model BatchResponse
//
//	@Description	Response payload containing results from batch processing multiple API requests.
//	@example		{
//	  "successCount": 2,
//	  "failureCount": 1,
//	  "results": [
//	    {"id": "req-1", "status": 201, "headers": {"Content-Type": "application/json"}, "body": {"id": "uuid-1", "legalName": "Acme Corp"}},
//	    {"id": "req-2", "status": 200, "headers": {"Content-Type": "application/json"}, "body": {"items": [], "page": 1, "limit": 10}},
//	    {"id": "req-3", "status": 400, "error": {"code": "0047", "title": "Bad Request", "message": "Invalid input"}}
//	  ]
//	}
type BatchResponse struct {
	// Number of requests that completed successfully (2xx status codes)
	// example: 2
	SuccessCount int `json:"successCount" example:"2"`

	// Number of requests that failed (non-2xx status codes)
	// example: 1
	FailureCount int `json:"failureCount" example:"1"`

	// Array of response items, one for each request in the batch
	Results []BatchResponseItem `json:"results"`
} //	@name	BatchResponse

// BatchResponseItem represents a single response within a batch.
//
// swagger:model BatchResponseItem
//
//	@Description	A single API response from a batch operation.
//	@example		{
//	  "id": "req-1",
//	  "status": 201,
//	  "headers": {"Content-Type": "application/json", "X-Request-Id": "abc123-def456"},
//	  "body": {"id": "uuid-1", "legalName": "Acme Corp"}
//	}
type BatchResponseItem struct {
	// The ID from the corresponding request item
	// example: req-1
	ID string `json:"id" example:"req-1"`

	// HTTP status code for this response
	// example: 201
	Status int `json:"status" example:"201"`

	// Response headers from the individual request
	// required: false
	Headers map[string]string `json:"headers,omitempty"`

	// Response body (present on success)
	Body json.RawMessage `json:"body,omitempty"`

	// Error details (present on failure)
	Error *BatchItemError `json:"error,omitempty"`
} //	@name	BatchResponseItem

// BatchItemError represents an error for a single item in a batch response.
//
// swagger:model BatchItemError
//
//	@Description	Error details for a failed request within a batch operation.
//	@example		{
//	  "code": "0047",
//	  "title": "Bad Request",
//	  "message": "Invalid input: field 'legalName' is required"
//	}
type BatchItemError struct {
	// Error code identifying the specific error
	// example: 0047
	Code string `json:"code" example:"0047"`

	// Human-readable error title
	// example: Bad Request
	Title string `json:"title" example:"Bad Request"`

	// Detailed error message
	// example: Invalid input: field 'legalName' is required
	Message string `json:"message" example:"Invalid input: field 'legalName' is required"`
} //	@name	BatchItemError
