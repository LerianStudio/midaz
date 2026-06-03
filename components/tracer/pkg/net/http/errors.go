// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	var (
		notFoundErr      *pkg.EntityNotFoundError
		conflictErr      *pkg.EntityConflictError
		validationErr    *pkg.ValidationError
		unprocessableErr *pkg.UnprocessableOperationError
		unauthorizedErr  *pkg.UnauthorizedError
		forbiddenErr     *pkg.ForbiddenError
		knownFieldsErr   *pkg.ValidationKnownFieldsError
		unknownFieldsErr *pkg.ValidationUnknownFieldsError
		responseErr      *pkg.ResponseError
	)

	switch {
	case errors.As(err, &notFoundErr):
		return NotFound(c, notFoundErr.Code, notFoundErr.Title, notFoundErr.Message)
	case errors.As(err, &conflictErr):
		return Conflict(c, conflictErr.Code, conflictErr.Title, conflictErr.Message)
	case errors.As(err, &knownFieldsErr):
		return BadRequest(c, *knownFieldsErr)
	case errors.As(err, &unknownFieldsErr):
		return BadRequest(c, *unknownFieldsErr)
	case errors.As(err, &validationErr):
		return BadRequest(c, pkg.ValidationKnownFieldsError{
			Code:    validationErr.Code,
			Title:   validationErr.Title,
			Message: validationErr.Message,
			Fields:  nil,
		})
	case errors.As(err, &unprocessableErr):
		return UnprocessableEntity(c, unprocessableErr.Code, unprocessableErr.Title, unprocessableErr.Message)
	case errors.As(err, &unauthorizedErr):
		return Unauthorized(c, unauthorizedErr.Code, unauthorizedErr.Title, unauthorizedErr.Message)
	case errors.As(err, &forbiddenErr):
		return Forbidden(c, forbiddenErr.Code, forbiddenErr.Title, forbiddenErr.Message)
	case errors.As(err, &responseErr):
		return JSONResponseError(c, *responseErr)
	default:
		// ValidateInternalError always returns an InternalServerError
		var internalErr *pkg.InternalServerError
		if errors.As(pkg.ValidateInternalError(err, ""), &internalErr) {
			return InternalServerError(c, internalErr.Code, internalErr.Title, internalErr.Message)
		}

		// Fallback uses centralized constants for consistency
		return InternalServerError(c, constant.ErrInternalServer.Error(), "Internal Server Error",
			"The server encountered an unexpected error. Please try again later or contact support.")
	}
}
