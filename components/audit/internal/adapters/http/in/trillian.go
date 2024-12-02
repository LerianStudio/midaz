package in

import (
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/operation"
	"github.com/LerianStudio/midaz/components/audit/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type TrillianHandler struct {
	UseCase *services.UseCase
}

func (th *TrillianHandler) CheckEntry(c *fiber.Ctx) error {
	return http.OK(c, "ok")
}

func (th *TrillianHandler) AuditLogs(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_Logs")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	auditInfo, err := th.UseCase.GetAuditInfo(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve audit info", err)

		logger.Errorf("Failed to retrieve audit info: %v", err.Error())

		return http.WithError(c, err) // TODO: error message
	}

	validations := make([]services.HashValidation, 0)

	for key, value := range auditInfo.Operations {
		v, err := th.UseCase.ValidatedLogHash(ctx, auditInfo.TreeID, value)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve log validation", err)

			logger.Errorf("Failed to retrieve log validation: %v", err.Error())

			return http.WithError(c, err) // TODO: error message
		}

		if v.WasTempered {
			logger.Warnf("Log for %v has been tampered! Expected: %x, Found: %x\n", key, v.ExpectedHash, v.CalculatedHash)
		}

		v.OperationID = key

		validations = append(validations, *v)
	}

	return http.OK(c, validations)
}

func (th *TrillianHandler) ReadLogs(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.read_logs")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	auditInfo, err := th.UseCase.GetAuditInfo(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve audit info", err)

		logger.Errorf("Failed to retrieve audit info: %v", err.Error())

		return http.WithError(c, err) // TODO: error message
	}

	operations := make([]operation.Operation, 0)

	for _, value := range auditInfo.Operations {
		o, err := th.UseCase.GetLogByHash(ctx, auditInfo.TreeID, value)
		if err != nil {
			return err
		}
		operations = append(operations, o)
	}

	return http.OK(c, operations)
}
