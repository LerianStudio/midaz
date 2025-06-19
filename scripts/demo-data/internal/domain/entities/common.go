package entities

import "errors"

// Common domain errors
var (
	ErrEntityNotFound       = errors.New("entity not found")
	ErrValidationFailed     = errors.New("validation failed")
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrConfigurationInvalid = errors.New("configuration is invalid")
)

// Status represents entity status information
type Status struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}
