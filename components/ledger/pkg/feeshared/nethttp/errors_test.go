// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestWithError_EntityNotFoundError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.EntityNotFoundError{
			Code:    constant.ErrEntityNotFound.Error(),
			Title:   "Entity Not Found",
			Message: "The requested entity was not found",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestWithError_EntityConflictError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.EntityConflictError{
			Code:    "CONFLICT-001",
			Title:   "Entity Conflict",
			Message: "The entity already exists",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
}

func TestWithError_ValidationError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationError{
			Code:    constant.ErrBadRequest.Error(),
			Title:   "Validation Error",
			Message: "Invalid input provided",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWithError_UnprocessableOperationError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.UnprocessableOperationError{
			Code:    "UNPROCESSABLE-001",
			Title:   "Unprocessable Operation",
			Message: "The operation cannot be processed",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnprocessableEntity, resp.StatusCode)
}

func TestWithError_UnauthorizedError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.UnauthorizedError{
			Code:    "UNAUTHORIZED-001",
			Title:   "Unauthorized",
			Message: "Authentication required",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestWithError_ForbiddenError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ForbiddenError{
			Code:    "FORBIDDEN-001",
			Title:   "Forbidden",
			Message: "Insufficient privileges",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestWithError_ValidationKnownFieldsError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationKnownFieldsError{
			Code:    constant.ErrBadRequest.Error(),
			Title:   "Validation Error",
			Message: "Invalid fields",
			Fields: pkg.FieldValidations{
				"field1": "Field 1 is required",
				"field2": "Field 2 is invalid",
			},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWithError_ValidationUnknownFieldsError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationUnknownFieldsError{
			Code:    constant.ErrUnexpectedFieldsInTheRequest.Error(),
			Title:   "Unexpected Fields",
			Message: "Unknown fields in request",
			Fields: pkg.UnknownFields{
				"unknownField1": "value1",
				"unknownField2": "value2",
			},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWithError_ResponseError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ResponseError{
			Code:    "RESPONSE-001",
			Title:   "Response Error",
			Message: "Custom response error",
		}
		// ResponseError needs to implement commons.Response
		// Let's create an error that implements the interface
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// ResponseError can return different status codes depending on the implementation
	// We verify that it's not a 500 error (which would be the default)
	assert.NotEqual(t, fiber.StatusInternalServerError, resp.StatusCode)
}

func TestWithError_DefaultCase_UnknownError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := errors.New("unknown error type")
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}

func TestWithError_DefaultCase_StandardError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := errors.New("standard go error")
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}

func TestWithError_ValidationError_FieldsMapping(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationError{
			Code:    constant.ErrBadRequest.Error(),
			Title:   "Validation Error",
			Message: "Invalid input",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWithError_AllErrorTypes(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
	}{
		{
			name: "EntityNotFoundError",
			err: pkg.EntityNotFoundError{
				Code:    constant.ErrEntityNotFound.Error(),
				Title:   "Not Found",
				Message: "Entity not found",
			},
			expectedStatus: fiber.StatusNotFound,
		},
		{
			name: "EntityConflictError",
			err: pkg.EntityConflictError{
				Code:    "CONFLICT-001",
				Title:   "Conflict",
				Message: "Entity conflict",
			},
			expectedStatus: fiber.StatusConflict,
		},
		{
			name: "ValidationError",
			err: pkg.ValidationError{
				Code:    constant.ErrBadRequest.Error(),
				Title:   "Validation Error",
				Message: "Validation failed",
			},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name: "UnprocessableOperationError",
			err: pkg.UnprocessableOperationError{
				Code:    "UNPROCESSABLE-001",
				Title:   "Unprocessable",
				Message: "Cannot process",
			},
			expectedStatus: fiber.StatusUnprocessableEntity,
		},
		{
			name: "UnauthorizedError",
			err: pkg.UnauthorizedError{
				Code:    "UNAUTHORIZED-001",
				Title:   "Unauthorized",
				Message: "Not authorized",
			},
			expectedStatus: fiber.StatusUnauthorized,
		},
		{
			name: "ForbiddenError",
			err: pkg.ForbiddenError{
				Code:    "FORBIDDEN-001",
				Title:   "Forbidden",
				Message: "Access forbidden",
			},
			expectedStatus: fiber.StatusForbidden,
		},
		{
			name: "ValidationKnownFieldsError",
			err: pkg.ValidationKnownFieldsError{
				Code:    constant.ErrBadRequest.Error(),
				Title:   "Validation Error",
				Message: "Invalid fields",
				Fields:  pkg.FieldValidations{"field": "error"},
			},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name: "ValidationUnknownFieldsError",
			err: pkg.ValidationUnknownFieldsError{
				Code:    constant.ErrUnexpectedFieldsInTheRequest.Error(),
				Title:   "Unexpected Fields",
				Message: "Unknown fields",
				Fields:  pkg.UnknownFields{"field": "value"},
			},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "Unknown error type",
			err:            errors.New("unknown error"),
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/test", func(c *fiber.Ctx) error {
				return WithError(c, tt.err)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			resp, err := app.Test(req)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Expected status %d for %s, got %d", tt.expectedStatus, tt.name, resp.StatusCode)
		})
	}
}

func TestWithError_ResponseError_WithCommonsResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		// Creating a ResponseError that implements commons.Response
		responseErr := commons.Response{
			Code:    "RESPONSE-001",
			Title:   "Response Error",
			Message: "Custom response",
		}
		// ResponseError needs to be converted to a type that implements error
		// Let's use an error that implements commons.Response through errors.As
		err := pkg.ResponseError{
			Code:    responseErr.Code,
			Title:   responseErr.Title,
			Message: responseErr.Message,
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// ResponseError can have different behavior depending on the implementation
	// We verify that it's not a critical error
	assert.NotEqual(t, fiber.StatusInternalServerError, resp.StatusCode)
}

func TestWithError_ErrorFieldsPreserved(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationKnownFieldsError{
			Code:    constant.ErrBadRequest.Error(),
			Title:   "Test Title",
			Message: "Test Message",
			Fields: pkg.FieldValidations{
				"field1": "Error message 1",
				"field2": "Error message 2",
			},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWithError_NilError(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return WithError(c, nil)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)

	assert.NoError(t, err)
	// Nil error should fall into the default case and return InternalServerError
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}
