// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreatePortfolio creates a new portfolio and persists it in the repository.
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreatePortfolioInput) (_ *mmodel.Portfolio, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_portfolio")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_portfolio", start, err)
	}()

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	portfolioID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate portfolio ID", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate portfolio ID", libLog.Err(err))

		return nil, err
	}

	now := time.Now()
	portfolio := &mmodel.Portfolio{
		ID:             portfolioID.String(),
		EntityID:       cpi.EntityID,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create portfolio", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create portfolio", libLog.Err(err))

		return nil, err
	}

	uc.emitPortfolioCreatedEvent(ctx, span, logger, port)

	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityPortfolio, port.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create portfolio metadata", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create portfolio metadata", libLog.Err(err))

		return nil, err
	}

	port.Metadata = metadata

	return port, nil
}

// emitPortfolioCreatedEvent publishes the portfolio.created event for a
// successfully persisted portfolio. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// The persisted database mutation is durable; this helper does not make broker delivery transactional.
//
// Anchor: invoked immediately after PortfolioRepo.Create succeeds and
// before CreateOnboardingMetadata runs, so a downstream Mongo failure
// cannot mask the event.
//
// Wire-format mapping lives in pkg/streaming/events/portfolio_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitPortfolioCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, p *mmodel.Portfolio) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.PortfolioCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewPortfolioCreated(p).ToEmitRequest(tenantID, p.CreatedAt)
		})
}
