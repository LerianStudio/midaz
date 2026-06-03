// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"

	"github.com/LerianStudio/reporter/pkg"

	"github.com/gofiber/fiber/v2"
)

// Unauthorized sends an HTTP 401 Unauthorized response with a custom code, title and message.
func Unauthorized(c *fiber.Ctx, code, title, message string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// Forbidden sends an HTTP 403 Forbidden response with a custom code, title and message.
func Forbidden(c *fiber.Ctx, code, title, message string) error {
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// BadRequest sends an HTTP 400 Bad Request response with a custom body.
// Plain string values are wrapped in a message envelope. Struct types
// with their own JSON serialization are passed through unchanged.
func BadRequest(c *fiber.Ctx, s any) error {
	if str, ok := s.(string); ok {
		s = fiber.Map{"message": str}
	}

	if errValue, ok := s.(error); ok {
		payload, err := json.Marshal(errValue)
		if err != nil || string(payload) == "{}" || string(payload) == "null" {
			s = fiber.Map{"message": errValue.Error()}
		}
	}

	// Verify the value is JSON-serializable before passing to Fiber.
	// Non-serializable types (channels, funcs) would cause a 500 instead of 400.
	if _, err := json.Marshal(s); err != nil {
		s = fiber.Map{"message": "bad request"}
	}

	return c.Status(fiber.StatusBadRequest).JSON(s)
}

// NotFound sends an HTTP 404 Not Found response with a custom code, title and message.
func NotFound(c *fiber.Ctx, code, title, message string) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// Conflict sends an HTTP 409 Conflict response with a custom code, title and message.
func Conflict(c *fiber.Ctx, code, title, message string) error {
	return c.Status(fiber.StatusConflict).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// UnprocessableEntity sends an HTTP 422 Unprocessable Entity response with a custom code, title and message.
func UnprocessableEntity(c *fiber.Ctx, code, title, message string) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// InternalServerError sends an HTTP 500 Internal Server Error response.
func InternalServerError(c *fiber.Ctx, code, title, message string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"code":    code,
		"title":   title,
		"message": message,
	})
}

// JSONResponseError sends a JSON formatted error response with a custom error struct.
// Note: This uses project-level pkg.ResponseError (not commons.Response) because the
// type includes a Code int field for HTTP status, which differs from lib-commons' Response type.
// This is an accepted deviation documented for future migration.
func JSONResponseError(c *fiber.Ctx, err pkg.ResponseError) error {
	return c.Status(err.Code).JSON(err)
}
