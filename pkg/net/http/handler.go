package http

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/trace"
)

// Ping returns HTTP Status 200 with response "pong".
func Ping(c *fiber.Ctx) error {
	if err := c.SendString("healthy"); err != nil {
		log.Print(err.Error())
	}

	return nil
}

// Version returns HTTP Status 200 with given version.
func Version(c *fiber.Ctx) error {
	return OK(c, fiber.Map{
		"version":     utils.GetenvOrDefault("VERSION", "0.0.0"),
		"requestDate": time.Now().UTC(),
	})
}

// Welcome returns HTTP Status 200 with service info.
func Welcome(service string, description string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service":     service,
			"description": description,
		})
	}
}

// NotImplementedEndpoint returns HTTP 501 with not implemented message.
func NotImplementedEndpoint(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "Not implemented yet"})
}

// File servers a specific file.
func File(filePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.SendFile(filePath)
	}
}

// ExtractTokenFromHeader extracts the authentication token from the Authorization header.
// It handles both "Bearer TOKEN" format and raw token format.
func ExtractTokenFromHeader(c *fiber.Ctx) string {
	authHeader := c.Get(fiber.HeaderAuthorization)

	if authHeader == "" {
		return ""
	}

	splitToken := strings.Split(authHeader, " ")

	if len(splitToken) > 1 && strings.EqualFold(splitToken[0], "bearer") {
		return strings.TrimSpace(splitToken[1])
	}

	if len(splitToken) > 0 {
		return strings.TrimSpace(splitToken[0])
	}

	return ""
}

// HandleFiberError handles errors for Fiber, properly unwrapping errors to check for fiber.Error
func HandleFiberError(c *fiber.Ctx, err error) error {
	// Safely end spans if user context exists
	ctx := c.UserContext()
	if ctx != nil {
		// End the span immediately instead of in a goroutine to ensure prompt completion
		trace.SpanFromContext(ctx).End()
	}

	// Default error handling
	code := fiber.StatusInternalServerError

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	if code == fiber.StatusInternalServerError {
		// Log the actual error for debugging purposes.
		log.Printf("handler error on %s %s: %v", c.Method(), c.Path(), err)

		return c.Status(code).JSON(fiber.Map{
			"error": http.StatusText(code),
		})
	}

	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
	})
}
