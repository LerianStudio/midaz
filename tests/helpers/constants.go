package helpers

// HTTP status codes for integration tests.
// Using named constants improves readability and maintainability.
const (
	// Success status codes
	HTTPStatusOK        = 200 // Standard response for successful HTTP requests
	HTTPStatusCreated   = 201 // Request succeeded and a new resource was created
	HTTPStatusNoContent = 204 // Request succeeded but no content to return

	// Client error status codes
	HTTPStatusBadRequest = 400 // Request contains invalid syntax or cannot be fulfilled
	HTTPStatusForbidden  = 403 // Server understood request but refuses to authorize
	HTTPStatusNotFound   = 404 // Server cannot find the requested resource
	HTTPStatusConflict   = 409 // Request conflicts with current state of target resource
)
