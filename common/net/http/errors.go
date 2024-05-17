package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/gofiber/fiber/v2"
)

// ResponseError is a struct used to return errors to the client.
type ResponseError struct {
	Code    int     `json:"code,omitempty"`
	Message string  `json:"message,omitempty"`
	Origin  *string `json:"origin,omitempty"`
}

// Error returns the message of the ResponseError.
//
// No parameters.
// Returns a string.
func (r ResponseError) Error() string {
	return r.Message
}

// ValidationError records an error indicating an entity was not found in any case that caused it.
type ValidationError struct {
	EntityType string           `json:"entityType,omitempty"`
	Title      string           `json:"title,omitempty"`
	Code       string           `json:"code,omitempty"`
	Message    string           `json:"message,omitempty"`
	Fields     FieldValidations `json:"fields,omitempty"`
}

// Error returns the error message for a ValidationError.
//
// No parameters.
// Returns a string.
func (r ValidationError) Error() string {
	return r.Message
}

// FieldValidations is a map of fields and their validation errors.
type FieldValidations map[string]string

// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	switch e := err.(type) {
	case common.EntityNotFoundError:
		return NotFound(c, e.Code, e.Message)
	case common.EntityConflictError:
		return Conflict(c, e.Code, e.Message)
	case common.ValidationError:
		return BadRequest(c, ValidationError{
			Code:    e.Code,
			Message: e.Message,
			Fields:  nil,
		})
	case common.UnprocessableOperationError:
		return UnprocessableEntity(c, e.Code, e.Message)
	case common.UnauthorizedError:
		return Unauthorized(c, e.Code, e.Error())
	case common.ForbiddenError:
		return Forbidden(c, e.Message)
	case *ValidationError, ValidationError:
		return BadRequest(c, e)
	case ResponseError:
		rErr, _ := err.(ResponseError)
		return JSONResponseError(c, rErr)
	default:
		return InternalServerError(c, e.Error())
	}
}
