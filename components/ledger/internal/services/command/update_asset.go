// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateAssetByID updates an asset from the repository by the given ID.
func (uc *UseCase) UpdateAssetByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, uii *mmodel.UpdateAssetInput) (*mmodel.Asset, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_asset_by_id")
	defer span.End()

	asset := &mmodel.Asset{
		Name:   uii.Name,
		Status: uii.Status,
	}

	assetUpdated, err := uc.AssetRepo.Update(ctx, organizationID, ledgerID, id, asset)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error updating asset on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset)

			logger.Log(ctx, libLog.LevelWarn, "Asset ID not found", libLog.String("asset_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update asset on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update asset on repo by id", err)

		return nil, err
	}

	uc.emitAssetUpdatedEvent(ctx, span, logger, assetUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityAsset, id.String(), uii.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error updating metadata", libLog.Err(err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	assetUpdated.Metadata = metadataUpdated

	return assetUpdated, nil
}

// emitAssetUpdatedEvent publishes the asset.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the AssetRepo.Update success branch and the
// metadata-write call in UpdateAssetByID, so a downstream Mongo failure
// cannot mask the event.
//
// Caller invariant: assetUpdated must be the post-commit value returned
// by AssetRepo.Update — i.e. the row state scanned from the RETURNING
// clause. The repo guarantees identity (ID, OrganizationID, LedgerID,
// Type, Code) and the persisted UpdatedAt reflect the row state, so
// this function does not merge against the pre-update record.
//
// Wire-format mapping lives in pkg/streaming/events/asset_updated.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAssetUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, a *mmodel.Asset) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AssetUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAssetUpdated(a).ToEmitRequest(tenantID, a.UpdatedAt)
		})
}
