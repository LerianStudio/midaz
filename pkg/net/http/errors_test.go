package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithError_ValidationKnownFieldsError_ValueType(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		// Return value type (not pointer)
		err := pkg.ValidationKnownFieldsError{
			Code:    "0009",
			Title:   "Missing Fields in Request",
			Message: "Required fields are missing",
			Fields:  pkg.FieldValidations{"name": "name is a required field"},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request for ValidationKnownFieldsError value type")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "0009")
}

func TestWithError_ValidationKnownFieldsError_PointerType(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		// Return pointer type (as ValidateStruct does)
		err := &pkg.ValidationKnownFieldsError{
			Code:    "0009",
			Title:   "Missing Fields in Request",
			Message: "Required fields are missing",
			Fields:  pkg.FieldValidations{"name": "name is a required field"},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request for ValidationKnownFieldsError pointer type")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "0009")
}

func TestWithError_ValidationUnknownFieldsError_ValueType(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationUnknownFieldsError{
			Code:    "0053",
			Title:   "Unexpected Fields in the Request",
			Message: "The request body contains more fields than expected",
			Fields:  pkg.UnknownFields{"extra_field": "value"},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request for ValidationUnknownFieldsError value type")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "0053")
}

func TestWithError_ValidationUnknownFieldsError_PointerType(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := &pkg.ValidationUnknownFieldsError{
			Code:    "0053",
			Title:   "Unexpected Fields in the Request",
			Message: "The request body contains more fields than expected",
			Fields:  pkg.UnknownFields{"extra_field": "value"},
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request for ValidationUnknownFieldsError pointer type")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "0053")
}

func TestWithError_EntityNotFoundError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.EntityNotFoundError{
			Code:    "0007",
			Title:   "Entity Not Found",
			Message: "No entity was found for the given ID",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Expected 404 Not Found for EntityNotFoundError")
}

func TestWithError_ValidationError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.ValidationError{
			Code:    "0004",
			Title:   "Code Uppercase Requirement",
			Message: "The code must be in uppercase",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected 400 Bad Request for ValidationError")
}

func TestWithError_EntityConflictError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.EntityConflictError{
			Code:    "0001",
			Title:   "Duplicate Ledger Error",
			Message: "A ledger with the name already exists",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode, "Expected 409 Conflict for EntityConflictError")
}

func TestWithError_UnprocessableOperationError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := pkg.UnprocessableOperationError{
			Code:    "0018",
			Title:   "Insufficient Funds Error",
			Message: "The transaction could not be completed due to insufficient funds",
		}
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "Expected 422 Unprocessable Entity for UnprocessableOperationError")
}

func TestWithError_UnknownError_ReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		err := assert.AnError // generic error that doesn't match any known type
		return WithError(c, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Expected 500 Internal Server Error for unknown error type")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "0046") // ErrInternalServer code
}
