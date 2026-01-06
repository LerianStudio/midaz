package in

import (
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	"github.com/gofiber/fiber/v2"
)

// AdminHandler handles administrative operations.
type AdminHandler struct {
	Resolver poolmanager.Resolver
}

// InvalidateTenantCache invalidates the cached configuration for a tenant.
// @Summary Invalidate tenant cache
// @Description Forces re-fetching of tenant configuration from Tenant Service
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /admin/cache/tenants/{id}/invalidate [post]
func (h *AdminHandler) InvalidateTenantCache(c *fiber.Ctx) error {
	tenantID := c.Params("id")
	if tenantID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "tenant ID is required",
		})
	}

	if h.Resolver != nil {
		h.Resolver.InvalidateCache(tenantID)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":   "tenant cache invalidated",
		"tenant_id": tenantID,
	})
}

// InvalidateAllTenantCache invalidates all cached tenant configurations.
// @Summary Invalidate all tenant cache
// @Description Forces re-fetching of all tenant configurations
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /admin/cache/tenants/invalidate [post]
func (h *AdminHandler) InvalidateAllTenantCache(c *fiber.Ctx) error {
	if h.Resolver != nil {
		h.Resolver.InvalidateCacheAll()
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "all tenant caches invalidated",
	})
}
