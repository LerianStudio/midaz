// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeletePackageByID delete a package from the repository
func (uc *UseCase) DeletePackageByID(ctx context.Context, id, organizationID uuid.UUID) (err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_package_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "fees", "delete_package", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	// Resolve the package's ledger BEFORE deleting so the cache can be
	// invalidated by its (org,ledger) key. SoftDelete is keyed only by
	// (id,org), so the ledger is not otherwise available. A miss here is
	// best-effort: the cache only needs invalidation when caching is enabled,
	// and a stale entry self-heals at the sentinel TTL.
	ledgerID, ledgerKnown := uc.resolvePackageLedger(ctx, logger, id, organizationID)

	if err := uc.packageRepo.SoftDelete(ctx, id, organizationID); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete package on repo by id", err)

		return err
	}

	if ledgerKnown {
		uc.invalidatePackageCache(ctx, logger, organizationID, ledgerID)
	}

	return nil
}

// resolvePackageLedger reads the package's ledger ID for cache invalidation.
// Returns (ledgerID, true) on success and (uuid.Nil, false) when caching is
// disabled or the lookup fails — both cases skip invalidation. It does NOT
// affect the delete's own not-found semantics: SoftDelete remains the
// authoritative existence check.
func (uc *UseCase) resolvePackageLedger(ctx context.Context, logger libLog.Logger, id, organizationID uuid.UUID) (uuid.UUID, bool) {
	if uc.PackageCache == nil {
		return uuid.Nil, false
	}

	amountData, err := uc.packageRepo.FindFeesAndAmountDataByPackageID(ctx, organizationID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to resolve package ledger for cache invalidation", libLog.Err(err))

		return uuid.Nil, false
	}

	return amountData.LedgerID, true
}
