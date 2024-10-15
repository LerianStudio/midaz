package http

import (
	"errors"

	"github.com/LerianStudio/midaz/common"
	"github.com/gofiber/fiber/v2"
)

// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	switch e := err.(type) {
	case common.EntityNotFoundError:
		return NotFound(c, e.Code, e.Title, e.Message)
	case common.EntityConflictError:
		return Conflict(c, e.Code, e.Title, e.Message)
	case common.ValidationError:
		return BadRequest(c, common.ValidationKnownFieldsError{
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
	case common.ValidationKnownFieldsError, common.ValidationUnknownFieldsError:
		return BadRequest(c, e)
	case common.ResponseError:
		var rErr common.ResponseError
		_ = errors.As(err, &rErr)

		return JSONResponseError(c, rErr)
	default:
		var iErr common.InternalServerError
		_ = errors.As(common.ValidateInternalError(err, ""), &iErr)

		return InternalServerError(c, iErr.Code, iErr.Title, iErr.Message)
	}
}
