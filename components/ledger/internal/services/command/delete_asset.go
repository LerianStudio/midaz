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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteAssetByID deletes an asset from the repository by IDs.
func (uc *UseCase) DeleteAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_asset_by_id")
	defer span.End()

	asset, err := uc.AssetRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset)

			logger.Log(ctx, libLog.LevelWarn, "Asset ID not found", libLog.String("asset_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get asset on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get asset on repo by id", err)

		logger.Log(ctx, libLog.LevelError, "Error getting asset")

		return err
	}

	aAlias := constant.DefaultExternalAccountAliasPrefix + asset.Code

	acc, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, []string{aAlias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve asset external account", err)

		logger.Log(ctx, libLog.LevelError, "Error retrieving asset external account")

		return err
	}

	if len(acc) > 0 {
		err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, nil, uuid.MustParse(acc[0].ID))
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete asset external account", err)

			logger.Log(ctx, libLog.LevelError, "Error deleting asset external account")

			return err
		}
	}

	if err := uc.AssetRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAssetIDNotFound, constant.EntityAsset)

			logger.Log(ctx, libLog.LevelWarn, "Asset ID not found", libLog.String("asset_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete asset on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete asset on repo by id", err)

		logger.Log(ctx, libLog.LevelError, "Error deleting asset")

		return err
	}

	uc.emitAssetDeletedEvent(ctx, span, logger, asset, time.Now())

	return nil
}

// emitAssetDeletedEvent publishes the asset.deleted event for a
// successfully soft-deleted asset. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after AssetRepo.Delete succeeds.
// AssetRepo.Delete does not return the post-delete record, so the
// payload sources identity from the pre-delete record (asset) and
// stamps deletedAt with the wall-clock instant captured by the caller.
// The PG deleted_at column is set by the same wall clock at row-update
// time, so the values are effectively identical up to clock skew.
//
// The cascade-delete of the implicit external account earlier in this
// use case goes through AccountRepo directly — NOT through
// UseCase.DeleteAccountByID — so it produces no account.deleted event.
//
// Wire-format mapping lives in pkg/streaming/events/asset_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAssetDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, a *mmodel.Asset, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AssetDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAssetDeleted(a.ID, a.OrganizationID, a.LedgerID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
