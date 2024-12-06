package in

import (
	"github.com/LerianStudio/midaz/pkg"
	"os"

	"github.com/LerianStudio/midaz/components/audit/api"

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
			if value := os.Getenv(env); !pkg.IsNilOrEmpty(&value) {
				if env == "SWAGGER_HOST" && pkg.ValidateServerAddress(value) == "" {
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
