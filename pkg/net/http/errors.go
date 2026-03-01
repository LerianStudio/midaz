// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// WithError returns an error with the given status code and message.
func WithError(c *fiber.Ctx, err error) error {
	{
		var (
			e   pkg.EntityNotFoundError
			e1  pkg.EntityConflictError
			e2  pkg.ValidationError
			e3  pkg.UnprocessableOperationError
			e4  pkg.UnauthorizedError
			e5  pkg.ForbiddenError
			e6  pkg.ValidationKnownFieldsError
			e7  pkg.ValidationUnknownFieldsError
			e8  pkg.ResponseError
			e9  libCommons.Response
			e10 pkg.InternalServerError
			e11 pkg.ServiceUnavailableError
		)

		switch {
		case errors.As(err, &e):
			return NotFound(c, e.Code, e.Title, e.Message)
		case errors.As(err, &e1):
			return Conflict(c, e1.Code, e1.Title, e1.Message)
		case errors.As(err, &e2):
			return BadRequest(c, pkg.ValidationKnownFieldsError{Code: e2.Code, Title: e2.Title, Message: e2.Message, Fields: nil})
		case errors.As(err, &e3):
			return UnprocessableEntity(c, e3.Code, e3.Title, e3.Message)
		case errors.As(err, &e4):
			return Unauthorized(c, e4.Code, e4.Title, e4.Message)
		case errors.As(err, &e5):
			return Forbidden(c, e5.Code, e5.Title, e5.Message)
		case errors.As(err, &e6), errors.As(err, &e7):
			return BadRequest(c, e6)
		case errors.As(err, &e8):
			var rErr pkg.ResponseError

			_ = errors.As(err, &rErr)

			return JSONResponseError(c, rErr)
		case errors.As(err, &e9):
			return handleResponseError(c, e9)
		case errors.As(err, &e10):
			return InternalServerError(c, e10.Code, e10.Title, e10.Message)
		case errors.As(err, &e11):
			if e11.Code == constant.ErrConsumerLagStaleBalance.Error() {
				c.Set("Retry-After", "1")
			}

			return ServiceUnavailable(c, e11.Code, e11.Title, e11.Message)
		default:
			return pkg.ValidateInternalError(err, "") //nolint:wrapcheck
		}
	}
}

// handleResponseError routes a libCommons.Response to the appropriate HTTP status handler.
func handleResponseError(c *fiber.Ctx, e9 libCommons.Response) error {
	switch e9.Code {
	case libConstants.ErrInsufficientFunds.Error(), libConstants.ErrAccountIneligibility.Error():
		return UnprocessableEntity(c, e9.Code, e9.Title, e9.Message)
	case libConstants.ErrAssetCodeNotFound.Error():
		return NotFound(c, e9.Code, e9.Title, e9.Message)
	case libConstants.ErrOverFlowInt64.Error():
		return InternalServerError(c, e9.Code, e9.Title, e9.Message)
	default:
		return BadRequest(c, pkg.ValidationKnownFieldsError{Code: e9.Code, Title: e9.Title, Message: e9.Message})
	}
}
