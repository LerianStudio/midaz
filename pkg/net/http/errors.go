// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/gofiber/fiber/v2"
)

// WithError returns an error with the given status code and message.
//
// Typed platform errors are resolved via errors.As so a wrapped error is still
// classified to its proper status. Business errors remain returned unwrapped by
// convention (E2); this is defensive hardening, not a license to wrap them.
//
// Resolution is order-dependent: the first matching arm in declaration order
// wins, and because every platform error type has an Unwrap, errors.As walks the
// whole chain. Nesting one platform error inside another sibling class therefore
// makes the OUTERMOST class drive the status — do not nest platform errors.
func WithError(c *fiber.Ctx, err error) error {
	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return NotFound(c, notFoundErr.Code, notFoundErr.Title, notFoundErr.Message)
	}

	var conflictErr pkg.EntityConflictError
	if errors.As(err, &conflictErr) {
		return Conflict(c, conflictErr.Code, conflictErr.Title, conflictErr.Message)
	}

	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return BadRequest(c, pkg.ValidationKnownFieldsError{
			Code:    validationErr.Code,
			Title:   validationErr.Title,
			Message: validationErr.Message,
			Fields:  nil,
		})
	}

	var unprocessableErr pkg.UnprocessableOperationError
	if errors.As(err, &unprocessableErr) {
		return UnprocessableEntity(c, unprocessableErr.Code, unprocessableErr.Title, unprocessableErr.Message)
	}

	var unauthorizedErr pkg.UnauthorizedError
	if errors.As(err, &unauthorizedErr) {
		return Unauthorized(c, unauthorizedErr.Code, unauthorizedErr.Title, unauthorizedErr.Message)
	}

	var forbiddenErr pkg.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return Forbidden(c, forbiddenErr.Code, forbiddenErr.Title, forbiddenErr.Message)
	}

	var knownFieldsErr pkg.ValidationKnownFieldsError
	if errors.As(err, &knownFieldsErr) {
		return BadRequest(c, knownFieldsErr)
	}

	var unknownFieldsErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &unknownFieldsErr) {
		return BadRequest(c, unknownFieldsErr)
	}

	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		return JSONResponseError(c, responseErr)
	}

	var commonsResponse libCommons.Response
	if errors.As(err, &commonsResponse) {
		switch commonsResponse.Code {
		case libConstants.ErrInsufficientFunds.Error(), libConstants.ErrAccountIneligibility.Error():
			return UnprocessableEntity(c, commonsResponse.Code, commonsResponse.Title, commonsResponse.Message)
		case libConstants.ErrAssetCodeNotFound.Error():
			return NotFound(c, commonsResponse.Code, commonsResponse.Title, commonsResponse.Message)
		case libConstants.ErrOverFlowInt64.Error():
			return InternalServerError(c, commonsResponse.Code, commonsResponse.Title, commonsResponse.Message)
		default:
			return BadRequest(c, pkg.ValidationKnownFieldsError{
				Code:    commonsResponse.Code,
				Title:   commonsResponse.Title,
				Message: commonsResponse.Message,
			})
		}
	}

	var internalErr pkg.InternalServerError
	if errors.As(err, &internalErr) {
		return InternalServerError(c, internalErr.Code, internalErr.Title, internalErr.Message)
	}

	var failedPreconditionErr pkg.FailedPreconditionError
	if errors.As(err, &failedPreconditionErr) {
		return InternalServerError(c, failedPreconditionErr.Code, failedPreconditionErr.Title, failedPreconditionErr.Message)
	}

	var unavailableErr pkg.ServiceUnavailableError
	if errors.As(err, &unavailableErr) {
		return ServiceUnavailable(c, unavailableErr.Code, unavailableErr.Title, unavailableErr.Message)
	}

	return pkg.ValidateInternalError(err, "")
}
