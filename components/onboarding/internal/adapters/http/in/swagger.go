// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/api"
	"github.com/gofiber/fiber/v2"
)

// WithSwaggerEnvConfig sets the Swagger configuration for the API documentation from environment variables if they are set.
func WithSwaggerEnvConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
		envVars := map[string]*string{
			"SWAGGER_TITLE":       &api.SwaggerInfoonboarding.Title,
			"SWAGGER_DESCRIPTION": &api.SwaggerInfoonboarding.Description,
			"SWAGGER_VERSION":     &api.SwaggerInfoonboarding.Version,
			"SWAGGER_HOST":        &api.SwaggerInfoonboarding.Host,
			"SWAGGER_BASE_PATH":   &api.SwaggerInfoonboarding.BasePath,
			"SWAGGER_LEFT_DELIM":  &api.SwaggerInfoonboarding.LeftDelim,
			"SWAGGER_RIGHT_DELIM": &api.SwaggerInfoonboarding.RightDelim,
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
			api.SwaggerInfoonboarding.Schemes = []string{schemes}
		}

		return c.Next()
	}
}
