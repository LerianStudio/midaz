// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"net/http"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"

	"github.com/LerianStudio/lib-commons/v5/commons"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/gofiber/fiber/v2"
)

// WithError returns an error with the given status code and message.
// Uses errors.As() for proper wrapped error support instead of type switch.
func WithError(c *fiber.Ctx, err error) error {
	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return commonsHttp.Respond(c, http.StatusNotFound, pkg.ResponseError{
			Code:    notFoundErr.Code,
			Title:   notFoundErr.Title,
			Message: notFoundErr.Message,
		})
	}

	var conflictErr pkg.EntityConflictError
	if errors.As(err, &conflictErr) {
		return commonsHttp.Respond(c, http.StatusConflict, pkg.ResponseError{
			Code:    conflictErr.Code,
			Title:   conflictErr.Title,
			Message: conflictErr.Message,
		})
	}

	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return commonsHttp.Respond(c, http.StatusBadRequest, pkg.ValidationKnownFieldsError{
			Code:    validationErr.Code,
			Title:   validationErr.Title,
			Message: validationErr.Message,
			Fields:  nil,
		})
	}

	var unprocessableErr pkg.UnprocessableOperationError
	if errors.As(err, &unprocessableErr) {
		return commonsHttp.Respond(c, http.StatusUnprocessableEntity, pkg.ResponseError{
			Code:    unprocessableErr.Code,
			Title:   unprocessableErr.Title,
			Message: unprocessableErr.Message,
		})
	}

	var unauthorizedErr pkg.UnauthorizedError
	if errors.As(err, &unauthorizedErr) {
		return commonsHttp.Respond(c, http.StatusUnauthorized, pkg.ResponseError{
			Code:    unauthorizedErr.Code,
			Title:   unauthorizedErr.Title,
			Message: unauthorizedErr.Message,
		})
	}

	var forbiddenErr pkg.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return commonsHttp.Respond(c, http.StatusForbidden, pkg.ResponseError{
			Code:    forbiddenErr.Code,
			Title:   forbiddenErr.Title,
			Message: forbiddenErr.Message,
		})
	}

	var knownFieldsErr pkg.ValidationKnownFieldsError
	if errors.As(err, &knownFieldsErr) {
		return commonsHttp.Respond(c, http.StatusBadRequest, knownFieldsErr)
	}

	var unknownFieldsErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &unknownFieldsErr) {
		return commonsHttp.Respond(c, http.StatusBadRequest, unknownFieldsErr)
	}

	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		var rErr commons.Response
		if errors.As(err, &rErr) {
			return commonsHttp.Respond(c, http.StatusBadRequest, rErr)
		}

		return commonsHttp.Respond(c, http.StatusBadRequest, commons.Response{
			Code:    responseErr.Code,
			Title:   responseErr.Title,
			Message: responseErr.Message,
		})
	}

	var internalErr pkg.InternalServerError
	if errors.As(err, &internalErr) {
		return commonsHttp.Respond(c, http.StatusInternalServerError, pkg.ResponseError{
			Code:    internalErr.Code,
			Title:   internalErr.Title,
			Message: internalErr.Message,
		})
	}

	var iErr pkg.InternalServerError

	_ = errors.As(pkg.ValidateInternalError(err, ""), &iErr)

	return commonsHttp.Respond(c, http.StatusInternalServerError, pkg.ResponseError{
		Code:    iErr.Code,
		Title:   iErr.Title,
		Message: iErr.Message,
	})
}
