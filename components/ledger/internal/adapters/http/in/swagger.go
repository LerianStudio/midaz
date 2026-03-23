// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/gofiber/fiber/v2"
)

// WithSwaggerEnvConfig sets the Swagger configuration for the API documentation from environment variables if they are set.
func WithSwaggerEnvConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
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

			for _, s := range strings.Split(schemes, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					parsed = append(parsed, s)
				}
			}

			if len(parsed) > 0 {
				api.SwaggerInfo.Schemes = parsed
			}
		}

		return c.Next()
	}
}
