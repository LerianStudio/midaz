package http

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// BasicAuthFunc represents a func which returns if a username and password was authenticated or not.
// It returns true if authenticated, and false when not authenticated.
type BasicAuthFunc func(username, password string) bool

// FixedBasicAuthFunc is a fixed username and password to use as BasicAuthFunc.
func FixedBasicAuthFunc(username, password string) BasicAuthFunc {
	return func(user, pass string) bool {
		if subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1 && subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1 {
			return true
		}

		return false
	}
}

// WithBasicAuth creates a basic authentication middleware.
func WithBasicAuth(f BasicAuthFunc, realm string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			return unauthorizedResponse(c, realm)
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Basic" {
			return unauthorizedResponse(c, realm)
		}

		cred, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return unauthorizedResponse(c, realm)
		}

		pair := strings.SplitN(string(cred), ":", 2)
		if len(pair) != 2 {
			return unauthorizedResponse(c, realm)
		}

		if f(pair[0], pair[1]) {
			return c.Next()
		}

		return unauthorizedResponse(c, realm)
	}
}

func unauthorizedResponse(c *fiber.Ctx, realm string) error {
	c.Set("WWW-Authenticate", `Basic realm="`+realm+`"`)

	return Unauthorized(c, "0042", "Invalid Token", "The provided token is expired, invalid or malformed. Please provide a valid token and try again.")
}
