// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"os"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// LedgerHandler struct contains a ledger use case for managing ledger related operations.
type LedgerHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createLedger/updateLedger/... methods below own the span, the service call
// and the success log. They take primitive args (parsed UUIDs, the already-decoded
// payload, the query map) so BOTH transports feed them: the Fiber wrappers pull
// those from *fiber.Ctx (Locals + the WithBody-decoded payload + c.Queries) and the
// Huma handlers (ledger_handler_huma.go) pull them from the request envelope. Every
// canonical Midaz error the cores return is rendered by the caller — http.WithError
// on the Fiber path, http.HumaProblem on the Huma path — so code + HTTP status are
// identical across both transports (no native Huma 422).

// createLedger owns the span + service call + success log for an already-decoded
// payload. Body decode+validation happens BEFORE this core (Fiber: WithBody
// decorator; Huma: http.DecodeAndValidate), so create is identical across transports.
func (handler *LedgerHandler) createLedger(ctx context.Context, organizationID uuid.UUID, payload *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_ledger")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create a ledger", payload)
	recordSafePayloadAttributes(span, payload)

	ledger, err := handler.Command.CreateLedger(ctx, organizationID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create ledger on command", err)

		return nil, err
	}

	return ledger, nil
}

// getLedgerByID retrieves a single ledger.
func (handler *LedgerHandler) getLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_ledger_by_id")
	defer span.End()

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve ledger on query", err)

		return nil, err
	}

	return ledger, nil
}

// getAllLedgers binds the query map imperatively (http.ValidateParameters — the
// SAME binder the Fiber path used), enforces the ledger-specific status allowlist
// and the metadata/name-filter mutual exclusion, then returns the assembled
// pagination envelope. Every rejection is a canonical 400 (no native Huma 422).
func (handler *LedgerHandler) getAllLedgers(ctx context.Context, organizationID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_ledgers")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	if headerParams.Status != nil && !isValidStatus(*headerParams.Status, ledgerAllowedStatuses) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLedger, "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: invalid ledger status", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate ledger status query parameter", libLog.String("status", *headerParams.Status), libLog.Err(err))

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		if headerParams.HasNameFilters() {
			err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityLedger, "metadata cannot be combined with name filters (name)")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: metadata and name filters are mutually exclusive", err)

			return http.Pagination{}, err
		}

		ledgers, err := handler.Query.GetAllMetadataLedgers(ctx, organizationID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all ledgers by metadata", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(ledgers)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	ledgers, err := handler.Query.GetAllLedgers(ctx, organizationID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all ledgers on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(ledgers)

	return pagination, nil
}

// updateLedger owns the span + service call + success log for an already-decoded
// payload (see createLedger for the decode split across transports).
func (handler *LedgerHandler) updateLedger(ctx context.Context, organizationID, id uuid.UUID, payload *mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update ledger", payload)
	recordSafePayloadAttributes(span, payload)

	ledger, err := handler.Command.UpdateLedgerByID(ctx, organizationID, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update ledger on command", err)

		return nil, err
	}

	return ledger, nil
}

// deleteLedger removes a ledger. The production-environment guard (ENV_NAME) is
// enforced HERE so both transports refuse the delete identically (canonical 0008 /
// 403 forbidden), not just on the Fiber path.
func (handler *LedgerHandler) deleteLedger(ctx context.Context, organizationID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_ledger_by_id")
	defer span.End()

	if os.Getenv("ENV_NAME") == "production" {
		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, constant.EntityLedger)

		handleSpanByErrorClass(span, "Failed to remove ledger on command", err)

		return err
	}

	if err := handler.Command.DeleteLedgerByID(ctx, organizationID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to remove ledger on command", err)

		return err
	}

	return nil
}

// countLedgers returns the total ledger count for the organization.
func (handler *LedgerHandler) countLedgers(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_ledgers")
	defer span.End()

	count, err := handler.Query.CountLedgers(ctx, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count ledgers", err)

		return 0, err
	}

	return count, nil
}

// getLedgerSettings returns the parsed settings for a ledger.
func (handler *LedgerHandler) getLedgerSettings(ctx context.Context, organizationID, id uuid.UUID) (mmodel.LedgerSettings, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_ledger_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", id.String()),
	)

	ledgerSettings, err := handler.Query.GetParsedLedgerSettings(ctx, organizationID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get ledger settings", err)

		return mmodel.LedgerSettings{}, err
	}

	return ledgerSettings, nil
}

// updateLedgerSettings owns the span + the schema-aware merge-patch service call.
//
// LANDMINE: the settings body is a free-form map[string]any, NOT a validated
// struct — the allowlist merge-patch (unknown fields -> 0147, wrong types -> 0148)
// lives in Command.UpdateLedgerSettings (via mmodel.ValidateSettings). Those are
// canonical business errors classified to a 400, so the caller renders them
// identically on both transports (Fiber: WithError; Huma: HumaProblem -> 400
// problem+json) — never a native Huma 422. Body decode happens BEFORE this core
// (Fiber: WithBody(new(map[string]any)); Huma: http.DecodeAndValidate into a map),
// so the null-byte/depth/key-count guards stay byte-identical too.
func (handler *LedgerHandler) updateLedgerSettings(ctx context.Context, organizationID, id uuid.UUID, settings map[string]any) (mmodel.LedgerSettings, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger_settings")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", id.String()),
	)

	updatedSettings, err := handler.Command.UpdateLedgerSettings(ctx, organizationID, id, settings)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger settings", err)

		return mmodel.LedgerSettings{}, err
	}

	return mmodel.ParseLedgerSettings(updatedSettings), nil
}

// CreateLedger is a method that creates Ledger information.
//
// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the
// handler methods directly; each pulls the transport inputs from *fiber.Ctx
// (Locals set by ParseUUIDPathParameters, the WithBody-decoded payload) and
// delegates to the shared core. The swaggo doc-comments are preserved verbatim
// (the migration is ADDITIVE; swaggo is unchanged) so the generated api/ spec keeps
// its per-op security. NOTE: once RegisterLedgerRoutesToApp is wired, the LIVE
// ledger routes are Huma (see ledger_handler_huma.go); these Fiber wrappers are the
// inline routes.go handlers and keep compiling until the integration task.
func (handler *LedgerHandler) CreateLedger(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledger, err := handler.createLedger(ctx, organizationID, i.(*mmodel.CreateLedgerInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, ledger)
}

// GetLedgerByID is a method that retrieves Ledger information by a given id.
func (handler *LedgerHandler) GetLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledger, err := handler.getLedgerByID(ctx, organizationID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, ledger)
}

// GetAllLedgers is a method that retrieves all ledgers.
func (handler *LedgerHandler) GetAllLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllLedgers(ctx, organizationID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// UpdateLedger is a method that updates Ledger information.
func (handler *LedgerHandler) UpdateLedger(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledger, err := handler.updateLedger(ctx, organizationID, id, p.(*mmodel.UpdateLedgerInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, ledger)
}

// DeleteLedgerByID is a method that removes Ledger information by a given id.
func (handler *LedgerHandler) DeleteLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deleteLedger(ctx, organizationID, id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountLedgers is a method that returns the total count of ledgers for a specific organization.
func (handler *LedgerHandler) CountLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	count, err := handler.countLedgers(ctx, organizationID)
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}

// GetLedgerSettings retrieves the settings for a specific ledger.
func (handler *LedgerHandler) GetLedgerSettings(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerSettings, err := handler.getLedgerSettings(ctx, organizationID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, ledgerSettings)
}

// UpdateLedgerSettings updates the settings for a specific ledger using schema-aware deep merge.
func (handler *LedgerHandler) UpdateLedgerSettings(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	// Defensive type assertion of the WithBody(new(map[string]any)) payload. The
	// allowlist merge-patch itself lives in the updateLedgerSettings core (via
	// Command.UpdateLedgerSettings -> mmodel.ValidateSettings). This BadRequest is
	// the sole HTTP-layer guard and stays Fiber-only; the Huma path decodes the map
	// directly from RawBody and cannot hit this branch.
	settings, ok := i.(*map[string]any)
	if !ok {
		return http.BadRequest(c, pkg.ValidateBusinessError(constant.ErrInvalidRequestBody, "settings"))
	}

	updatedSettings, err := handler.updateLedgerSettings(ctx, organizationID, id, *settings)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, updatedSettings)
}
