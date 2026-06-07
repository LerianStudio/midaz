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
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteOrganizationByID deletes an organization from the repository.
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.delete_organization_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "delete_organization", start, err)
	}()

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityOrganization)

			logger.Log(ctx, libLog.LevelWarn, "Organization ID not found", libLog.String("organization_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete organization on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete organization on repo by id", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete organization", libLog.Err(err))

		return err
	}

	deletedAt := time.Now()
	uc.emitOrganizationDeletedEvent(ctx, span, logger, id.String(), deletedAt)

	return nil
}

// emitOrganizationDeletedEvent publishes the organization.deleted event for a
// successfully soft-deleted organization. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitOrganizationDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.OrganizationDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewOrganizationDeleted(id, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
