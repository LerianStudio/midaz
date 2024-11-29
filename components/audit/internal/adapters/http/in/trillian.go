package in

import (
	"github.com/LerianStudio/midaz/components/audit/internal/services"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

type TrillianHandler struct {
	UseCase *services.UseCase
}

func (th *TrillianHandler) CheckEntry(c *fiber.Ctx) error {
	return http.OK(c, "ok")
}

func (th *TrillianHandler) AuditLogs(c *fiber.Ctx) error {
	return http.OK(c, "ok")
}

func (th *TrillianHandler) ReadLogs(c *fiber.Ctx) error {
	return http.OK(c, "ok")
}
