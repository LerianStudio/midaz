// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// PortfolioHandler struct contains a portfolio use case for managing portfolio related operations.
type PortfolioHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createPortfolio/updatePortfolio/... methods below own the span, imperative
// query binding, the service call and the success log. They take primitive args
// (parsed UUIDs, the decoded payload, the query map) so BOTH transports feed them:
// the Fiber wrappers pull those from *fiber.Ctx (Locals + WithBody payload +
// c.Queries) and the Huma handlers (portfolio_handler_huma.go) pull them from the
// request envelope. Every canonical Midaz error the cores return is rendered by the
// caller — http.WithError on the Fiber path, http.HumaProblem on the Huma path — so
// the code + HTTP status are identical across both transports.

// createPortfolio owns the span + service call + success log for an already-decoded
// payload. Body decode+validation happens BEFORE this core (Fiber: WithBody
// decorator; Huma: http.DecodeAndValidate(RawBody)).
func (handler *PortfolioHandler) createPortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_portfolio")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create a portfolio", payload)
	recordSafePayloadAttributes(span, payload)

	portfolio, err := handler.Command.CreatePortfolio(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Portfolio on command", err)

		return nil, err
	}

	return portfolio, nil
}

// getAllPortfolios binds the query map imperatively (http.ValidateParameters — the
// SAME binder the Fiber path used) so a bad query yields the canonical 400, then
// returns the assembled pagination envelope.
func (handler *PortfolioHandler) getAllPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_portfolios")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

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
		portfolios, err := handler.Query.GetAllMetadataPortfolios(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Portfolios on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(portfolios)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	portfolios, err := handler.Query.GetAllPortfolio(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Portfolios on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(portfolios)

	return pagination, nil
}

// getPortfolioByID retrieves a single portfolio.
func (handler *PortfolioHandler) getPortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_portfolio_by_id")
	defer span.End()

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Portfolio on query", err)

		return nil, err
	}

	return portfolio, nil
}

// updatePortfolio owns the span + service call + success log for an already-decoded
// payload (see createPortfolio for the decode split across transports).
func (handler *PortfolioHandler) updatePortfolio(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_portfolio")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update portfolio", payload)
	recordSafePayloadAttributes(span, payload)

	portfolio, err := handler.Command.UpdatePortfolioByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Portfolio on command", err)

		return nil, err
	}

	return portfolio, nil
}

// deletePortfolio removes a portfolio.
func (handler *PortfolioHandler) deletePortfolio(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_portfolio_by_id")
	defer span.End()

	if err := handler.Command.DeletePortfolioByID(ctx, organizationID, ledgerID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to remove Portfolio on command", err)

		return err
	}

	return nil
}

// countPortfolios returns the total portfolio count for the ledger.
func (handler *PortfolioHandler) countPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_portfolios")
	defer span.End()

	count, err := handler.Query.CountPortfolios(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count portfolios", err)

		return 0, err
	}

	return count, nil
}

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the handler
// methods directly; each pulls the transport inputs from *fiber.Ctx (Locals set by
// ParseUUIDPathParameters, the WithBody-decoded payload as `i`) and delegates to the
// shared core. NOTE: once wired, the LIVE portfolio routes are
// Huma (see portfolio_handler_huma.go + RegisterPortfolioRoutesToApp); these Fiber
// wrappers are not mounted by the unified server.

// CreatePortfolio is a method that creates portfolio information.
func (handler *PortfolioHandler) CreatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	portfolio, err := handler.createPortfolio(ctx, organizationID, ledgerID, i.(*mmodel.CreatePortfolioInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, portfolio)
}

// GetAllPortfolios is a method that retrieves all Portfolios.
func (handler *PortfolioHandler) GetAllPortfolios(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllPortfolios(ctx, organizationID, ledgerID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// GetPortfolioByID is a method that retrieves Portfolio information by a given id.
func (handler *PortfolioHandler) GetPortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	portfolio, err := handler.getPortfolioByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, portfolio)
}

// UpdatePortfolio is a method that updates Portfolio information.
func (handler *PortfolioHandler) UpdatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	portfolio, err := handler.updatePortfolio(ctx, organizationID, ledgerID, id, i.(*mmodel.UpdatePortfolioInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, portfolio)
}

// DeletePortfolioByID is a method that removes Portfolio information by a given ids.
func (handler *PortfolioHandler) DeletePortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deletePortfolio(ctx, organizationID, ledgerID, id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountPortfolios is a method that returns the total count of portfolios for a specific organization and ledger.
func (handler *PortfolioHandler) CountPortfolios(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	count, err := handler.countPortfolios(ctx, organizationID, ledgerID)
	if err != nil {
		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
