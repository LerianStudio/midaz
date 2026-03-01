// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

var swaggerConfigOnce sync.Once

// WithSwaggerEnvConfig returns a middleware that applies Swagger configuration
// from environment variables exactly once (on first request), ensuring thread-safe
// initialization without data races on the global api.SwaggerInfo.
func WithSwaggerEnvConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !utils.SwaggerEnabled() {
			return c.SendStatus(fiber.StatusNotFound)
		}

		token := utils.SwaggerRequestToken(c)
		if !utils.SwaggerTokenAuthorized(token) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		swaggerConfigOnce.Do(initSwaggerFromEnv)

		return c.Next()
	}
}

// initSwaggerFromEnv reads environment variables and applies them to api.SwaggerInfo.
// This function is called exactly once via sync.Once to avoid data races.
func initSwaggerFromEnv() {
	envVars := map[string]*string{
		"SWAGGER_TITLE":       &api.SwaggerInfo.Title,
		"SWAGGER_DESCRIPTION": &api.SwaggerInfo.Description,
		"SWAGGER_VERSION":     &api.SwaggerInfo.Version,
		"SWAGGER_HOST":        &api.SwaggerInfo.Host,
		"SWAGGER_BASE_PATH":   &api.SwaggerInfo.BasePath,
		"SWAGGER_LEFT_DELIM":  &api.SwaggerInfo.LeftDelim,
		"SWAGGER_RIGHT_DELIM": &api.SwaggerInfo.RightDelim,
	}

	for env, field := range envVars {
		if value := os.Getenv(env); !libCommons.IsNilOrEmpty(&value) {
			if env == "SWAGGER_HOST" && libCommons.ValidateServerAddress(value) == "" {
				continue
			}

			*field = value
		}
	}

	if schemes := os.Getenv("SWAGGER_SCHEMES"); schemes != "" {
		var parsed []string

		for s := range strings.SplitSeq(schemes, ",") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				parsed = append(parsed, trimmed)
			}
		}

		if len(parsed) > 0 {
			api.SwaggerInfo.Schemes = parsed
		}
	}
}
