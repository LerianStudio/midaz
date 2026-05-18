// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdatePortfolioByID updates a portfolio from the repository by the given ID.
func (uc *UseCase) UpdatePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, upi *mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_portfolio_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update portfolio %s", id.String()))

	portfolio := &mmodel.Portfolio{
		EntityID: upi.EntityID,
		Name:     upi.Name,
		Status:   upi.Status,
	}

	portfolioUpdated, err := uc.PortfolioRepo.Update(ctx, organizationID, ledgerID, id, portfolio)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating portfolio on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Portfolio ID not found: %s", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update portfolio on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update portfolio on repo by id", err)

		return nil, err
	}

	uc.emitPortfolioUpdatedEvent(ctx, span, logger, portfolioUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), id.String(), upi.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating metadata: %v", err))

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
// Caller invariant: portfolioUpdated must be the post-commit value
// returned by PortfolioRepo.Update — i.e. the row state scanned from
// the RETURNING clause. The repo guarantees identity (ID, OrganizationID,
// LedgerID) and the persisted UpdatedAt reflect the row state, so this
// function does not merge against the pre-update record.
//
// Wire-format mapping lives in pkg/streaming/events/portfolio_updated.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitPortfolioUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, p *mmodel.Portfolio) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.PortfolioUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewPortfolioUpdated(p).ToEmitRequest(tenantID, p.UpdatedAt)
		})
}
