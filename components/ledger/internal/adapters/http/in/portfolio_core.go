// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// PortfolioHandler struct contains a portfolio use case for managing portfolio related operations.
type PortfolioHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createPortfolio/updatePortfolio/... methods below own the span, imperative
// query binding and the service call. They take primitive args — parsed UUIDs,
// the decoded payload, the query map — so nothing transport-shaped
// reaches them; the handlers in portfolio_handler.go pull those out of the request
// envelope. Every canonical Midaz error a core returns is rendered by its caller via
// http.HumaProblem, which fixes the code + HTTP status.

// createPortfolio owns the span + service call for an already-decoded
// payload. Body decode+validation happens BEFORE this core (Fiber: WithBody
// decorator; Huma: http.DecodeAndValidate(RawBody)).
func (handler *PortfolioHandler) createPortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_portfolio")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	portfolio, err := handler.Command.CreatePortfolio(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Portfolio on command", err)

		return nil, err
	}

	return portfolio, nil
}

// getAllPortfolios binds the query map imperatively via http.ValidateParameters so a
// bad query yields the canonical 400, then returns the assembled pagination
// envelope.
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

// updatePortfolio owns the span + service call for an already-decoded
// payload (see createPortfolio for the decode split across transports).
func (handler *PortfolioHandler) updatePortfolio(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_portfolio")
	defer span.End()

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
