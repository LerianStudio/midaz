// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"

	"github.com/LerianStudio/reporter/pkg"

	"github.com/gofiber/fiber/v2"
)

// IsBusinessError checks if an error is a business/domain error (validation, not-found, conflict, etc.)
// as opposed to a technical/infrastructure error. Business errors should use HandleSpanBusinessErrorEvent
// to avoid polluting error rate metrics with expected business conditions.
func IsBusinessError(err error) bool {
	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return true
	}

	var conflictErr pkg.EntityConflictError
	if errors.As(err, &conflictErr) {
		return true
	}

	var validationKnownFieldsErr pkg.ValidationKnownFieldsError
	if errors.As(err, &validationKnownFieldsErr) {
		return true
	}

	var validationUnknownFieldsErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &validationUnknownFieldsErr) {
		return true
	}

	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return true
	}

	var unprocessableErr pkg.UnprocessableOperationError
	if errors.As(err, &unprocessableErr) {
		return true
	}

	var unauthorizedErr pkg.UnauthorizedError
	if errors.As(err, &unauthorizedErr) {
		return true
	}

	var forbiddenErr pkg.ForbiddenError

	return errors.As(err, &forbiddenErr)
}

// WithError returns an error with the given status code and message.
// It uses errors.As() to correctly identify domain error types even when
// they are wrapped with fmt.Errorf("%w") or multiple layers of wrapping.
func WithError(c *fiber.Ctx, err error) error {
	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return NotFound(c, notFoundErr.Code, notFoundErr.Title, notFoundErr.Message)
	}

	var conflictErr pkg.EntityConflictError
	if errors.As(err, &conflictErr) {
		return Conflict(c, conflictErr.Code, conflictErr.Title, conflictErr.Message)
	}

	var validationKnownFieldsErr pkg.ValidationKnownFieldsError
	if errors.As(err, &validationKnownFieldsErr) {
		return BadRequest(c, validationKnownFieldsErr)
	}

	var validationUnknownFieldsErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &validationUnknownFieldsErr) {
		return BadRequest(c, validationUnknownFieldsErr)
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

	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		return JSONResponseError(c, responseErr)
	}

	var iErr pkg.InternalServerError

	_ = errors.As(pkg.ValidateInternalError(err, ""), &iErr)

	return InternalServerError(c, iErr.Code, iErr.Title, iErr.Message)
}
