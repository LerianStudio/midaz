// Package in provides HTTP handlers for incoming requests to the transaction service.
// This file contains Swagger configuration middleware.
package in

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/api"
	"github.com/gofiber/fiber/v2"
)

// WithSwaggerEnvConfig creates middleware that configures Swagger from environment variables.
//
// This middleware dynamically configures Swagger API documentation based on environment
// variables, allowing different Swagger configurations for different deployment environments
// (development, staging, production) without code changes.
//
// Supported Environment Variables:
//   - SWAGGER_TITLE: API title
//   - SWAGGER_DESCRIPTION: API description
//   - SWAGGER_VERSION: API version
//   - SWAGGER_HOST: API host (validated for proper format)
//   - SWAGGER_BASE_PATH: API base path
//   - SWAGGER_LEFT_DELIM: Template left delimiter
//   - SWAGGER_RIGHT_DELIM: Template right delimiter
//   - SWAGGER_SCHEMES: API schemes (http, https)
//
// Returns:
//   - fiber.Handler: Middleware that configures Swagger before serving docs
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
			api.SwaggerInfo.Schemes = []string{schemes}
		}

		return c.Next()
	}
}
