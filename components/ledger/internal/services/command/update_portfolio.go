// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdatePortfolioByID updates a portfolio from the repository by the given ID.
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdatePortfolioInput) (_ *mmodel.Portfolio, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_portfolio_by_id")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_portfolio", start, err)
	}()

	portfolio := &mmodel.Portfolio{
		EntityID: upi.EntityID,
		Name:     upi.Name,
		Status:   upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, organizationID, ledgerID, id, portfolio)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, constant.EntityPortfolio)

			logger.Log(ctx, libLog.LevelWarn, "Portfolio ID not found", libLog.String("portfolio_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update portfolio on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update portfolio on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update portfolio on repo by id", libLog.Err(err))

		return nil, err
	}

	uc.emitPortfolioUpdatedEvent(ctx, span, logger, portfolioUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityPortfolio, id.String(), upi.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update portfolio metadata", libLog.Err(err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	portfolioUpdated.Metadata = metadataUpdated

	return portfolioUpdated, nil
}

// emitPortfolioUpdatedEvent publishes the portfolio.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the PortfolioRepo.Update success branch and
// the metadata-write call in UpdatePortfolioByID, so a downstream Mongo
// failure cannot mask the event.
//
// Caller invariant: p must be the value returned by PortfolioRepo.Update
// (post-commit), not the input struct. Specifically p.ID, p.UpdatedAt
// and the persisted EntityID/Name/Status must reflect the row state.
//
// Wire-format mapping lives in pkg/streaming/events/portfolio_updated.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitPortfolioUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, p *mmodel.Portfolio) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.PortfolioUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewPortfolioUpdated(p).ToEmitRequest(tenantID, p.UpdatedAt)
		})
}
