// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateAccountingRouteCache stores the msgpack-encoded action-aware cache of a
// transaction route under its organization key, with no expiry. When the route
// was created under a ledger, it also deletes the ledger-scoped key so a pod
// still reading that key reloads the route instead of serving the previous rule.
// Both writes are attempted; their failures are joined.
func (uc *UseCase) CreateAccountingRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route_cache")
	defer span.End()

	cacheBytes, err := route.ToCache().ToMsgpack()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert route to cache data", err)

		return err
	}

	setErr := uc.TransactionRedisRepo.SetBytes(ctx, utils.AccountingRoutesInternalKey(route.OrganizationID, route.ID), cacheBytes, 0)
	if setErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create transaction route cache", setErr)
	}

	legacyErr := uc.deleteLedgerAccountingRouteCache(ctx, route)
	if legacyErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete ledger-scoped transaction route cache", legacyErr)
	}

	return errors.Join(setErr, legacyErr)
}

// deleteLedgerAccountingRouteCache removes the ledger-scoped key of a route
// created under a ledger. A route created at organization level has no such key.
func (uc *UseCase) deleteLedgerAccountingRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	if route.LedgerID == nil {
		return nil
	}

	return uc.TransactionRedisRepo.Del(ctx, utils.LedgerAccountingRoutesInternalKey(route.OrganizationID, *route.LedgerID, route.ID))
}
