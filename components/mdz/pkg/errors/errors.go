package errors

import (
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/pkg"
)

// Unwrap returns the result of calling the Unwrap method on err, if err's
// type contains an Unwrap method returning error.
// Otherwise, Unwrap returns nil.
func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// Is reports whether any error in err's chain matches target.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's chain that matches target,
// and if so, sets target to that error value and returns true.
// Otherwise, it returns false.
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// New returns an error that formats as the given text.
func New(text string) error {
	return errors.New(text)
}

// Wrap wraps an error with a message to provide context
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Wrapf wraps an error with a formatted message to provide context
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

// CommandError wraps an error with command context for the CLI
func CommandError(command string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("command '%s' failed: %w", command, err)
}

// UserError creates a user-oriented error that includes a suggestion
func UserError(err error, suggestion string) error {
	if err == nil {
		return nil
	}
	
	if suggestion != "" {
		return fmt.Errorf("%w (suggestion: %s)", err, suggestion)
	}
	return err
}

// APIError creates an error for API-related issues
func APIError(endpoint string, statusCode int, err error) error {
	if err == nil {
		return nil
	}
	
	return pkg.HTTPError{
		Title:   "API Request Failed",
		Message: fmt.Sprintf("Request to %s failed with status %d: %s", endpoint, statusCode, err.Error()),
		Code:    fmt.Sprintf("HTTP_%d", statusCode),
		Err:     err,
	}
}

// ValidationError creates a standard validation error
func ValidationError(field, message string) error {
	return pkg.ValidationError{
		Title:   "Validation Error",
		Message: fmt.Sprintf("%s: %s", field, message),
		Code:    "VALIDATION_ERROR",
	}
}

// NotFoundError creates a standard not found error
func NotFoundError(entityType, id string) error {
	return pkg.EntityNotFoundError{
		EntityType: entityType,
		Title:      "Not Found",
		Message:    fmt.Sprintf("%s with ID '%s' was not found", entityType, id),
		Code:       "NOT_FOUND",
	}
}