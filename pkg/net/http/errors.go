// Package http provides HTTP utilities and helpers for the Midaz ledger system.
// This file contains error handling utilities for converting domain errors to HTTP responses.
package http

import (
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
)

// WithError converts domain errors to appropriate HTTP responses.
//
// This function is the central error handling mechanism for HTTP handlers. It performs
// type assertion on the error to determine its type, then calls the appropriate response
// function with the correct HTTP status code.
//
// Error Type to HTTP Status Code Mapping:
//   - pkg.EntityNotFoundError       -> 404 Not Found
//   - pkg.EntityConflictError       -> 409 Conflict
//   - pkg.ValidationError           -> 400 Bad Request
//   - pkg.UnprocessableOperationError -> 422 Unprocessable Entity
//   - pkg.UnauthorizedError         -> 401 Unauthorized
//   - pkg.ForbiddenError            -> 403 Forbidden
//   - pkg.ValidationKnownFieldsError -> 400 Bad Request
//   - pkg.ValidationUnknownFieldsError -> 400 Bad Request
//   - pkg.ResponseError             -> Variable (based on error code)
//   - pkg.InternalServerError       -> 500 Internal Server Error
//   - libCommons.Response           -> Variable (based on error code)
//   - Unknown errors                -> 500 Internal Server Error
//
// Parameters:
//   - c: Fiber context for the HTTP request
//   - err: The domain error to be converted to an HTTP response
//
// Returns:
//   - error: Fiber error (typically nil as response is sent)
//
// Example Usage:
//
//	account, err := service.GetAccount(id)
//	if err != nil {
//	    return http.WithError(c, err)
//	}
//	return http.OK(c, account)
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
	case libCommons.Response:
		switch e.Code {
		case libConstants.ErrInsufficientFunds.Error(), libConstants.ErrAccountIneligibility.Error():
			return UnprocessableEntity(c, e.Code, e.Title, e.Message)
		case libConstants.ErrAssetCodeNotFound.Error():
			return NotFound(c, e.Code, e.Title, e.Message)
		case libConstants.ErrOverFlowInt64.Error():
			return InternalServerError(c, e.Code, e.Title, e.Message)
		default:
			return BadRequest(c, pkg.ValidationKnownFieldsError{
				Code:    e.Code,
				Title:   e.Title,
				Message: e.Message,
			})
		}

	case pkg.InternalServerError:
		return InternalServerError(c, e.Code, e.Title, e.Message)
	default:
		return pkg.ValidateInternalError(err, "")
	}
}
