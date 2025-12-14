// Package http provides HTTP utilities for Fiber handlers.
//
// # Panic Recovery
//
// Functions in this package may panic on programming errors (e.g., missing middleware,
// wrong payload types). These panics are intentionally caught by Fiber's built-in
// recover middleware, which converts them to 500 Internal Server Error responses
// with proper logging. This design ensures:
//
//   - Fast-fail on wiring mistakes during development
//   - Safe error responses in production (no process crash)
//   - Rich context in logs for debugging
//
// See also: github.com/gofiber/fiber/v2/middleware/recover
package http

import (
	"fmt"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// LocalUUID safely extracts a uuid.UUID from c.Locals().
// Panics with rich context if the key is not set or is not a uuid.UUID.
//
// Example:
//
//	organizationID := http.LocalUUID(c, "organization_id")
func LocalUUID(c *fiber.Ctx, key string) uuid.UUID {
	val := c.Locals(key)
	assert.NotNil(val, "middleware must set locals key",
		"key", key,
		"path", c.Path(),
		"method", c.Method())

	id, ok := val.(uuid.UUID)
	assert.That(ok, "locals value must be uuid.UUID",
		"key", key,
		"actual_type", typeName(val),
		"path", c.Path())

	return id
}

// LocalUUIDOptional safely extracts a uuid.UUID from c.Locals(), returning uuid.Nil if not set.
// Panics if the key is set but is not a uuid.UUID.
//
// Example:
//
//	parentID := http.LocalUUIDOptional(c, "parent_id") // Returns uuid.Nil if not set
func LocalUUIDOptional(c *fiber.Ctx, key string) uuid.UUID {
	val := c.Locals(key)
	if val == nil {
		return uuid.Nil
	}

	id, ok := val.(uuid.UUID)
	assert.That(ok, "locals value must be uuid.UUID when set",
		"key", key,
		"actual_type", typeName(val),
		"path", c.Path())

	return id
}

// Payload asserts that the decoded payload has the expected type.
//
// WHY: WithBody ensures payload is non-nil, but wiring mistakes can still pass the wrong payload type.
// This helper turns a confusing type-assertion panic into a rich assertion failure.
//
// Example:
//
//	input := http.Payload[*transaction.CreateTransactionInput](c, p)
func Payload[T any](c *fiber.Ctx, p any) T {
	assert.NotNil(p, "payload must not be nil after validation",
		"path", c.Path(),
		"method", c.Method())

	payload, ok := p.(T)

	var zero T
	assert.That(ok, "payload has unexpected type",
		"expected_type", typeName(zero),
		"actual_type", typeName(p),
		"path", c.Path(),
		"method", c.Method())

	return payload
}

// LocalStringSlice safely extracts a []string from c.Locals().
// Panics with rich context if the key is not set or is not a []string.
//
// Example:
//
//	fieldsToRemove := http.LocalStringSlice(c, "patchRemove")
func LocalStringSlice(c *fiber.Ctx, key string) []string {
	val := c.Locals(key)
	assert.NotNil(val, "middleware must set locals key",
		"key", key,
		"path", c.Path(),
		"method", c.Method())

	slice, ok := val.([]string)
	assert.That(ok, "locals value must be []string",
		"key", key,
		"actual_type", typeName(val),
		"path", c.Path())

	return slice
}

// typeName returns the type name of a value for error messages
func typeName(v any) string {
	if v == nil {
		return "nil"
	}

	return fmt.Sprintf("%T", v)
}
