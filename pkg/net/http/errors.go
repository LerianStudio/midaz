package http

import (
	"errors"

	"github.com/LerianStudio/midaz/pkg"

	"github.com/gofiber/fiber/v2"
)

// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	switch e := err.(type) {
	case pkg.EntityNotFoundError:
		return NotFound(c, e.Code, e.Title, e.Message)
	case pkg.EntityConflictError:
		return Conflict(c, e.Code, e.Title, e.Message)
	case pkg.ValidationError:
		return BadRequest(c, pkg.ValidationKnownFieldsError{
			Code:    e.Code,
			Title:   e.Title,
			Message: e.Message,
			Fields:  nil,
		})
	case pkg.UnprocessableOperationError:
		return UnprocessableEntity(c, e.Code, e.Title, e.Message)
	case pkg.UnauthorizedError:
		return Unauthorized(c, e.Code, e.Title, e.Message)
	case pkg.ForbiddenError:
		return Forbidden(c, e.Code, e.Title, e.Message)
	case pkg.ValidationKnownFieldsError, pkg.ValidationUnknownFieldsError:
		return BadRequest(c, e)
	case pkg.ResponseError:
		var rErr pkg.ResponseError
		_ = errors.As(err, &rErr)

		return JSONResponseError(c, rErr)
	default:
		var iErr pkg.InternalServerError
		_ = errors.As(pkg.ValidateInternalError(err, ""), &iErr)

		return InternalServerError(c, iErr.Code, iErr.Title, iErr.Message)
	}
}
