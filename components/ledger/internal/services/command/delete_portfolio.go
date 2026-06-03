// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObs "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeletePortfolioByID deletes a portfolio from the repository by IDs.
func (uc *UseCase) DeletePortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_portfolio_by_id")
	defer span.End()

	if err := uc.PortfolioRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrPortfolioIDNotFound, constant.EntityPortfolio)

			logger.Log(ctx, libLog.LevelWarn, "Portfolio ID not found", libLog.String("portfolio_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete portfolio on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete portfolio on repo by id", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete portfolio", libLog.Err(err))

		return err
	}

	uc.emitPortfolioDeletedEvent(ctx, span, logger, id.String(), organizationID.String(), ledgerID.String(), time.Now())

	return nil
}

// emitPortfolioDeletedEvent publishes the portfolio.deleted event for a
// successfully soft-deleted portfolio. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after PortfolioRepo.Delete succeeds.
// PortfolioRepo.Delete does not return the post-delete record, so the
// payload sources identity from the use-case parameters (which match
// the request path) and stamps deletedAt with the wall-clock instant
// captured by the caller. The PG deleted_at column is set by the same
// wall clock at row-update time, so the values are effectively identical
// up to clock skew.
//
// Wire-format mapping lives in pkg/streaming/events/portfolio_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitPortfolioDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID, ledgerID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.PortfolioDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewPortfolioDeleted(id, organizationID, ledgerID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
