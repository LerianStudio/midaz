package in

import (
	"os"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/api"

	"github.com/gofiber/fiber/v2"
)

// WithSwaggerEnvConfig sets the Swagger configuration for the API documentation from environment variables if they are set.
func WithSwaggerEnvConfig() fiber.Handler {
	return func(c *fiber.Ctx) error {
		envVars := map[string]*string{
			"SWAGGER_TITLE":       &api.SwaggerInfocrm.Title,
			"SWAGGER_DESCRIPTION": &api.SwaggerInfocrm.Description,
			"SWAGGER_VERSION":     &api.SwaggerInfocrm.Version,
			"SWAGGER_HOST":        &api.SwaggerInfocrm.Host,
			"SWAGGER_BASE_PATH":   &api.SwaggerInfocrm.BasePath,
			"SWAGGER_LEFT_DELIM":  &api.SwaggerInfocrm.LeftDelim,
			"SWAGGER_RIGHT_DELIM": &api.SwaggerInfocrm.RightDelim,
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
			api.SwaggerInfocrm.Schemes = []string{schemes}
		}

		return c.Next()
	}
}
