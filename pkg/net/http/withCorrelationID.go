package http

import (
	"github.com/gofiber/fiber/v2"
	gid "github.com/google/uuid"
)

// WithCorrelationID creates a correlation id.
func WithCorrelationID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cid := gid.New().String()

		c.Set(HeaderCorrelationID, cid)
		c.Request().Header.Add(HeaderCorrelationID, cid)

		return c.Next()
	}
}
