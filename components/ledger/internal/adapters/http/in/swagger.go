package in

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/api"
	"github.com/gofiber/fiber/v2"
)

// WithSwaggerEnvConfig sets the Swagger configuration for the API documentation from environment variables if they are set.
func WithSwaggerEnvConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
		envVars := map[string]*string{
			"SWAGGER_TITLE":       &api.SwaggerInfo.Title,
			"SWAGGER_DESCRIPTION": &api.SwaggerInfo.Description,
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

		if version := os.Getenv("VERSION"); !libCommons.IsNilOrEmpty(&version) {
			api.SwaggerInfo.Version = version
		}

		if schemes := os.Getenv("SWAGGER_SCHEMES"); schemes != "" {
			api.SwaggerInfo.Schemes = []string{schemes}
		}

		return c.Next()
	}
}
