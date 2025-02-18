package in

import (
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

// AuditLogs compares leaf hash with a hash generated from the leaf value
//
//	@Summary		Audit logs by reference ID
//	@Description	Audit logs to check if any was tampered
//	@Tags			Audit
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			audit_id		path		string	true	"Audit ID"
//	@Success		200				{object}	HashValidationResponse
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/audit/{audit_id}/audit-logs [get]
func (th *TrillianHandler) AuditLogs(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.audit_Logs")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("audit_id").(uuid.UUID)

	auditInfo, err := th.UseCase.GetAuditInfo(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve audit info", err)

		logger.Errorf("Failed to retrieve audit info: %v", err.Error())

		return http.WithError(c, err)
	}

	validations := make([]HashValidationResponse, 0)

	for key, value := range auditInfo.Leaves {
		expectedHash, calculatedHash, isTampered, err := th.UseCase.ValidatedLogHash(ctx, auditInfo.TreeID, value)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve log validation", err)

			logger.Errorf("Failed to retrieve log validation: %v", err.Error())

			return http.WithError(c, err)
		}

		if isTampered {
			logger.Warnf("Log for %v has been tampered! Expected: %x, Found: %x\n", key, expectedHash, calculatedHash)
		}

		response := &HashValidationResponse{
			AuditID:        key,
			ExpectedHash:   expectedHash,
			CalculatedHash: calculatedHash,
			IsTampered:     isTampered,
		}

		validations = append(validations, *response)
	}

	return http.OK(c, validations)
}

// ReadLogs retrieves log values by the audit id
//
//	@Summary		Get 	logs by reference ID
//	@Description	Get log values from Trillian by reference ID
//	@Tags			Audit
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			audit_id		path		string	true	"Audit ID"
//	@Success		200				{object}	LogsResponse
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/audit/{audit_id}/read-logs [get]
func (th *TrillianHandler) ReadLogs(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.read_logs")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("audit_id").(uuid.UUID)

	auditInfo, err := th.UseCase.GetAuditInfo(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve audit info", err)

		logger.Errorf("Failed to retrieve audit info: %v", err.Error())

		return http.WithError(c, err)
	}

	leaves := make([]Leaf, 0)

	for _, value := range auditInfo.Leaves {
		logHash, logValue, err := th.UseCase.GetLogByHash(ctx, auditInfo.TreeID, value)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve log by hash", err)

			logger.Errorf("Failed to retrieve log by hash: %v", err.Error())

			return http.WithError(c, err)
		}

		leaves = append(leaves, Leaf{
			LeafID: logHash,
			Body:   logValue,
		})
	}

	response := &LogsResponse{
		TreeID: auditInfo.TreeID,
		Leaves: leaves,
	}

	return http.OK(c, response)
}
