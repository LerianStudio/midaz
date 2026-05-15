// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteOrganizationByID deletes an organization from the repository.
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.delete_organization_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Removing organization", libLog.String("organization_id", id.String()))

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
	if uc.Streaming == nil {
		return
	}

	event, buildErr := events.NewOrganizationDeleted(id, deletedAt).ToEvent(
		pkgStreaming.ResolveTenantID(ctx),
		uc.StreamingSource,
		deletedAt,
	)
	if buildErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build organization.deleted event", buildErr)
		logger.Log(ctx, libLog.LevelWarn, "Skipping organization.deleted emit; build failed", libLog.Err(buildErr))

		return
	}

	if emitErr := uc.Streaming.Emit(ctx, event); emitErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to emit organization.deleted", emitErr)
		logger.Log(ctx, libLog.LevelWarn, "Streaming emit failed for organization.deleted", libLog.Err(emitErr))
	}
}
