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
// shared core. The swaggo doc-comments below are preserved verbatim (the migration
// is ADDITIVE; swaggo is unchanged). NOTE: once wired, the LIVE portfolio routes are
// Huma (see portfolio_handler_huma.go + RegisterPortfolioRoutesToApp); these Fiber
// wrappers are not mounted by the unified server.

// CreatePortfolio is a method that creates portfolio information.
//
//	@Summary		Create a new portfolio
//	@Description	Creates a new portfolio within the specified ledger. Portfolios represent collections of accounts grouped for specific purposes such as business units, departments, or client portfolios.
//	@Tags			Portfolios
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			portfolio		body		mmodel.CreatePortfolioInput	true	"Portfolio details including name, optional entity ID, status, and metadata"
//	@Success		201				{object}	mmodel.Portfolio			"Successfully created portfolio"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Organization or ledger not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Portfolio with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios [post]
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
//
//	@Summary		List all portfolios
//	@Description	Returns a paginated list of portfolios within the specified ledger, optionally filtered by metadata, date range, and other criteria
//	@Tags			Portfolios
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string																false	"Request ID for tracing"
//	@Param			organization_id	path		string																true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string																true	"Ledger ID in UUID format"
//	@Param			metadata		query		string																false	"JSON string to filter portfolios by metadata fields"
//	@Param			entity_id		query		string																false	"Filter portfolios by entity ID"
//	@Param			status			query		string																false	"Filter portfolios by status"
//	@Param			limit			query		int																	false	"Maximum number of records to return per page"	default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int																	false	"Page number for pagination"					default(1)	minimum(1)
//	@Param			start_date		query		string																false	"Filter portfolios created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string																false	"Filter portfolios created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string																false	"Sort direction for results based on creation date"	Enums(asc,desc)
//	@Success		200				{object}	http.Pagination{items=[]mmodel.Portfolio}	"Successfully retrieved portfolios list"
//	@Failure		400				{object}	mmodel.Error														"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error														"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error														"Forbidden access"
//	@Failure		404				{object}	mmodel.Error														"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error														"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios [get]
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
//
//	@Summary		Retrieve a specific portfolio
//	@Description	Returns detailed information about a portfolio identified by its UUID within the specified ledger
//	@Tags			Portfolios
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string				false	"Request ID for tracing"
//	@Param			organization_id	path		string				true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string				true	"Ledger ID in UUID format"
//	@Param			portfolio_id				path		string				true	"Portfolio ID in UUID format"
//	@Success		200				{object}	mmodel.Portfolio	"Successfully retrieved portfolio"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Portfolio, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id} [get]
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
//
//	@Summary		Update a portfolio
//	@Description	Updates an existing portfolio's properties such as name, entity ID, status, and metadata within the specified ledger
//	@Tags			Portfolios
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			portfolio_id				path		string						true	"Portfolio ID in UUID format"
//	@Param			portfolio		body		mmodel.UpdatePortfolioInput	true	"Portfolio properties to update including name, entity ID, status, and optional metadata"
//	@Success		200				{object}	mmodel.Portfolio			"Successfully updated portfolio"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Portfolio, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Portfolio with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id} [patch]
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
//
//	@Summary		Delete a portfolio
//	@Description	Permanently removes a portfolio from the specified ledger. This operation cannot be undone.
//	@Tags			Portfolios
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Param			portfolio_id				path		string			true	"Portfolio ID in UUID format"
//	@Success		204				"Portfolio successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Portfolio, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Portfolio cannot be deleted due to existing dependencies"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id} [delete]
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
//
//	@Summary		Count total portfolios
//	@Description	Returns the total count of portfolios for a specific organization and ledger as a header without a response body
//	@Tags			Portfolios
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Success		204				"Successfully counted portfolios, total count available in X-Total-Count header"
//	@Failure		400				{object}	mmodel.Error	"Invalid UUID format"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/metrics/count [head]
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
