// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/api"
	"github.com/gofiber/fiber/v2"
)

// WithSwaggerEnvConfig sets the Swagger configuration for the API documentation from environment variables if they are set.
func WithSwaggerEnvConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
		envVars := map[string]*string{
			"SWAGGER_TITLE":       &api.SwaggerInfotransaction.Title,
			"SWAGGER_DESCRIPTION": &api.SwaggerInfotransaction.Description,
			"SWAGGER_VERSION":     &api.SwaggerInfotransaction.Version,
			"SWAGGER_HOST":        &api.SwaggerInfotransaction.Host,
			"SWAGGER_BASE_PATH":   &api.SwaggerInfotransaction.BasePath,
			"SWAGGER_LEFT_DELIM":  &api.SwaggerInfotransaction.LeftDelim,
			"SWAGGER_RIGHT_DELIM": &api.SwaggerInfotransaction.RightDelim,
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
			api.SwaggerInfotransaction.Schemes = []string{schemes}
		}

		return c.Next()
	}
}
