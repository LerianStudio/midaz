package mmigration

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// FiberHealthHandler returns a Fiber handler for migration health checks.
// Use this to add migration status to your service's health endpoint.
//
// Example usage in routes.go:
//
//	f.Get("/health/migrations", mmigration.FiberHealthHandler(migrationWrapper))
func FiberHealthHandler(checker HealthChecker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		status := checker.GetHealthStatus()

		statusCode := http.StatusOK
		if !status.Healthy {
			statusCode = http.StatusServiceUnavailable
		}

		return c.Status(statusCode).JSON(status)
	}
}

// FiberReadinessCheck returns true if migrations are healthy.
// Use this in readiness probe handlers.
//
// Example:
//
//	f.Get("/ready", func(c *fiber.Ctx) error {
//	    if !mmigration.FiberReadinessCheck(migrationWrapper) {
//	        return c.SendStatus(http.StatusServiceUnavailable)
//	    }
//	    return c.SendStatus(http.StatusOK)
//	})
func FiberReadinessCheck(checker HealthChecker) bool {
	return checker.GetHealthStatus().Healthy
}

// MigrationHealthResponse is the response structure for migration health.
// This matches the HealthStatus JSON output.
//
// swagger:response MigrationHealthResponse
// @Description Migration health status response
type MigrationHealthResponse struct {
	// in: body
	Body HealthStatus
}
