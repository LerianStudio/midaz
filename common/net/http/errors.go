package http

import (
	"errors"
	"github.com/LerianStudio/midaz/common"
	"github.com/gofiber/fiber/v2"
)

// ResponseError is a struct used to return errors to the client.
type ResponseError struct {
	Code    int    `json:"code,omitempty"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message,omitempty"`
}

// Error returns the message of the ResponseError.
//
// No parameters.
// Returns a string.
func (r ResponseError) Error() string {
	return r.Message
}

// ValidationKnownFieldsError records an error that occurred during a validation of known fields.
type ValidationKnownFieldsError struct {
	EntityType string           `json:"entityType,omitempty"`
	Title      string           `json:"title,omitempty"`
	Code       string           `json:"code,omitempty"`
	Message    string           `json:"message,omitempty"`
	Fields     FieldValidations `json:"fields,omitempty"`
}

// Error returns the error message for a ValidationKnownFieldsError.
//
// No parameters.
// Returns a string.
func (r ValidationKnownFieldsError) Error() string {
	return r.Message
}

// FieldValidations is a map of known fields and their validation errors.
type FieldValidations map[string]string

// ValidationUnknownFieldsError records an error that occurred during a validation of known fields.
type ValidationUnknownFieldsError struct {
	EntityType string        `json:"entityType,omitempty"`
	Title      string        `json:"title,omitempty"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
	Fields     UnknownFields `json:"fields,omitempty"`
}

// Error returns the error message for a ValidationUnknownFieldsError.
//
// No parameters.
// Returns a string.
func (r ValidationUnknownFieldsError) Error() string {
	return r.Message
}

// UnknownFields is a map of unknown fields and their error messages.
type UnknownFields map[string]any

// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	switch e := err.(type) {
	case common.EntityNotFoundError:
		return NotFound(c, e.Code, e.Title, e.Message)
	case common.EntityConflictError:
		return Conflict(c, e.Code, e.Title, e.Message)
	case common.ValidationError:
		return BadRequest(c, ValidationKnownFieldsError{
			Code:    e.Code,
			Title:   e.Title,
			Message: e.Message,
			Fields:  nil,
		})
	case common.UnprocessableOperationError:
		return UnprocessableEntity(c, e.Code, e.Title, e.Message)
	case common.UnauthorizedError:
		return Unauthorized(c, e.Code, e.Title, e.Message)
	case common.ForbiddenError:
		return Forbidden(c, e.Code, e.Title, e.Message)
	case *ValidationKnownFieldsError, ValidationKnownFieldsError:
		return BadRequest(c, e)
	case ResponseError:
		var rErr ResponseError
		_ = errors.As(err, &rErr)

		return JSONResponseError(c, rErr)
	default:
		var iErr common.InternalServerError
		_ = errors.As(common.ValidateInternalError(err, ""), &iErr)

		return InternalServerError(c, iErr.Code, iErr.Title, iErr.Message)
	}
}
