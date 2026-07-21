package buildinfo

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// VersionHandler returns a Fiber handler that responds 200 with the service
// version plus build provenance. It preserves the lib-commons Version wire
// shape for the version/requestDate fields and adds commit/buildTime/dirty.
// An empty version defaults to "0.0.0".
func VersionHandler(version string) fiber.Handler {
	if version == "" {
		version = "0.0.0"
	}

	return func(c *fiber.Ctx) error {
		info := Get()

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"version":     version,
			"requestDate": time.Now().UTC(),
			"commit":      info.Commit,
			"buildTime":   info.BuildTime,
			"dirty":       info.Dirty,
		})
	}
}
