package http

import (
	"net/http"
	"strconv"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/gofiber/fiber/v2"
)

// Unauthorized sends an HTTP 401 Unauthorized response with a custom code, title and message.
func Unauthorized(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// Forbidden sends an HTTP 403 Forbidden response with a custom code, title and message.
func Forbidden(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusForbidden).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// BadRequest sends an HTTP 400 Bad Request response with a custom body.
func BadRequest(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusBadRequest).JSON(s)
}

// Created sends an HTTP 201 Created response with a custom body.
func Created(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusCreated).JSON(s)
}

// OK sends an HTTP 200 OK response with a custom body.
func OK(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusOK).JSON(s)
}

// NoContent sends an HTTP 204 No Content response without anybody.
func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(http.StatusNoContent)
}

// Accepted sends an HTTP 202 Accepted response with a custom body.
func Accepted(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusAccepted).JSON(s)
}

// PartialContent sends an HTTP 206 Partial Content response with a custom body.
func PartialContent(c *fiber.Ctx, s any) error {
	return c.Status(http.StatusPartialContent).JSON(s)
}

// RangeNotSatisfiable sends an HTTP 416 Requested Range Not Satisfiable response.
func RangeNotSatisfiable(c *fiber.Ctx) error {
	return c.SendStatus(http.StatusRequestedRangeNotSatisfiable)
}

// NotFound sends an HTTP 404 Not Found response with a custom code, title and message.
func NotFound(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusNotFound).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// Conflict sends an HTTP 409 Conflict response with a custom code, title and message.
func Conflict(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusConflict).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// NotImplemented sends an HTTP 501 Not Implemented response with a custom message.
func NotImplemented(c *fiber.Ctx, message string) error {
	return c.Status(http.StatusNotImplemented).JSON(fiber.Map{
		"code":    http.StatusNotImplemented,
		"title":   "Not Implemented",
		"message": message,
	})
}

// UnprocessableEntity sends an HTTP 422 Unprocessable Entity response with a custom code, title and message.
func UnprocessableEntity(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusUnprocessableEntity).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// InternalServerError sends an HTTP 500 Internal Server Error response
func InternalServerError(c *fiber.Ctx, code, title, message string) error {
	return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// JSONResponseError sends a JSON formatted error response with a custom error struct.
func JSONResponseError(c *fiber.Ctx, err pkg.ResponseError) error {
	code, _ := strconv.Atoi(err.Code)

	return c.Status(code).JSON(err)
}

// JSONResponse sends a custom status code and body as a JSON response.
func JSONResponse(c *fiber.Ctx, status int, s any) error {
	return c.Status(status).JSON(s)
}
