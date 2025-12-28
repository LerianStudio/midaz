package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestApp() *fiber.App {
	return fiber.New()
}

func TestUnauthorized(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return Unauthorized(c, "AUTH001", "Unauthorized", "Invalid token")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "AUTH001", body["code"])
	assert.Equal(t, "Unauthorized", body["title"])
	assert.Equal(t, "Invalid token", body["message"])
}

func TestForbidden(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return Forbidden(c, "FORBID001", "Forbidden", "Access denied")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "FORBID001", body["code"])
	assert.Equal(t, "Forbidden", body["title"])
	assert.Equal(t, "Access denied", body["message"])
}

func TestBadRequest(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return BadRequest(c, map[string]string{"error": "validation failed"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "validation failed", body["error"])
}

func TestCreated(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return Created(c, map[string]string{"id": "123"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "123", body["id"])
}

func TestOK(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return OK(c, map[string]string{"status": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "success", body["status"])
}

func TestNoContent(t *testing.T) {
	app := setupTestApp()
	app.Delete("/test", func(c *fiber.Ctx) error {
		return NoContent(c)
	})

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestAccepted(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return Accepted(c, map[string]string{"status": "processing"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "processing", body["status"])
}

func TestPartialContent(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return PartialContent(c, map[string]any{"items": []string{"a", "b"}})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusPartialContent, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	items, ok := body["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestRangeNotSatisfiable(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return RangeNotSatisfiable(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, resp.StatusCode)
}

func TestNotFound(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return NotFound(c, "NOT001", "Not Found", "Resource not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "NOT001", body["code"])
	assert.Equal(t, "Not Found", body["title"])
	assert.Equal(t, "Resource not found", body["message"])
}

func TestConflict(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return Conflict(c, "CONF001", "Conflict", "Resource already exists")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "CONF001", body["code"])
	assert.Equal(t, "Conflict", body["title"])
	assert.Equal(t, "Resource already exists", body["message"])
}

func TestNotImplemented(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return NotImplemented(c, "Feature not available")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Code is int (501), not string
	assert.Equal(t, float64(http.StatusNotImplemented), body["code"])
	assert.Equal(t, "Not Implemented", body["title"])
	assert.Equal(t, "Feature not available", body["message"])
}

func TestUnprocessableEntity(t *testing.T) {
	app := setupTestApp()
	app.Post("/test", func(c *fiber.Ctx) error {
		return UnprocessableEntity(c, "UNP001", "Unprocessable", "Invalid entity")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "UNP001", body["code"])
	assert.Equal(t, "Unprocessable", body["title"])
	assert.Equal(t, "Invalid entity", body["message"])
}

func TestInternalServerError(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return InternalServerError(c, "INT001", "Internal Error", "Something went wrong")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "INT001", body["code"])
	assert.Equal(t, "Internal Error", body["title"])
	assert.Equal(t, "Something went wrong", body["message"])
}

func TestJSONResponseError(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponseError(c, pkg.ResponseError{
			Code:    "400",
			Title:   "Bad Request",
			Message: "Invalid input",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "400", body["code"])
	assert.Equal(t, "Bad Request", body["title"])
	assert.Equal(t, "Invalid input", body["message"])
}

func TestJSONResponseError_InvalidCode(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponseError(c, pkg.ResponseError{
			Code:    "invalid",
			Title:   "Error",
			Message: "Something wrong",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	// When code is invalid/unparseable, strconv.Atoi returns 0
	// Fiber treats Status(0) as 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestJSONResponse(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponse(c, http.StatusTeapot, map[string]string{"tea": "ready"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusTeapot, resp.StatusCode)

	var body map[string]string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ready", body["tea"])
}

func TestJSONResponse_WithSlice(t *testing.T) {
	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponse(c, http.StatusOK, []string{"item1", "item2", "item3"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []string
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Len(t, body, 3)
	assert.Equal(t, "item1", body[0])
}

func TestJSONResponse_WithStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	app := setupTestApp()
	app.Get("/test", func(c *fiber.Ctx) error {
		return JSONResponse(c, http.StatusOK, TestStruct{Name: "test", Value: 42})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body TestStruct
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "test", body.Name)
	assert.Equal(t, 42, body.Value)
}
