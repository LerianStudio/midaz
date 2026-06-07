// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"regexp"
	"strings"

	"github.com/LerianStudio/lib-commons/v5/commons"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	http "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
)

var (
	UUIDPathParameter = "id"

	// validStringPathParam defines the allowlist pattern for non-UUID string path parameters.
	// Starts with a letter (blocks path traversal), allows alphanumeric + underscore + hyphen, max 128 chars.
	validStringPathParam = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)
)

// SecurityHeaders returns a Fiber middleware that sets standard HTTP security
// headers on every response. The headers mitigate common browser-side attacks
// such as MIME-type sniffing, clickjacking, and reflected XSS.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "0")
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		return c.Next()
	}
}

// RecoverMiddleware returns a Fiber middleware that recovers from panics
// inside handlers, preventing a single panicking request from crashing the
// entire server process. It delegates to Fiber's built-in recover middleware.
func RecoverMiddleware() fiber.Handler {
	return recover.New()
}

// ParseUUIDPathParam returns a Fiber middleware that validates the named path
// parameter as a UUID. On success the parsed uuid.UUID is stored in
// c.Locals(paramName) for downstream handlers. On failure a 400 Bad Request
// response is returned with the standard ErrInvalidPathParameter error.
func ParseUUIDPathParam(paramName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pathParam := c.Params(paramName)

		if commons.IsNilOrEmpty(&pathParam) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", paramName)
			return http.WithError(c, err)
		}

		parsedPathUUID, errPath := uuid.Parse(pathParam)
		if errPath != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", paramName)
			return http.WithError(c, err)
		}

		c.Locals(paramName, parsedPathUUID)

		return c.Next()
	}
}

// ParsePathParametersUUID convert and validate if the path parameter is UUID.
// It validates the "id" path parameter. For other parameter names use
// ParseUUIDPathParam(paramName).
func ParsePathParametersUUID(c *fiber.Ctx) error {
	return ParseUUIDPathParam(UUIDPathParameter)(c)
}

// ParseStringPathParam returns a Fiber middleware that validates the named path
// parameter as a safe string identifier. On success the validated string is stored
// in c.Locals(paramName) for downstream handlers. On failure a 400 Bad Request
// response is returned with the standard ErrInvalidPathParameter error.
func ParseStringPathParam(paramName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pathParam := c.Params(paramName)

		if commons.IsNilOrEmpty(&pathParam) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", paramName)
			return http.WithError(c, err)
		}

		if !validStringPathParam.MatchString(pathParam) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", paramName)
			return http.WithError(c, err)
		}

		// Copy the string to detach from fasthttp's reusable buffer.
		// Without this, the string stored in Locals can be silently corrupted
		// when fasthttp reuses the request context for a subsequent request.
		c.Locals(paramName, strings.Clone(pathParam))

		return c.Next()
	}
}
