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

// DeleteTransactionRouteCache deletes the organization key of a transaction route
// and, when the route was created under a ledger, its ledger-scoped key. Both
// deletes are attempted; their failures are joined.
func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_cache")
	defer span.End()

	organizationErr := uc.TransactionRedisRepo.Del(ctx, utils.AccountingRoutesInternalKey(route.OrganizationID, route.ID))
	if organizationErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete transaction route cache", organizationErr)
	}

	legacyErr := uc.deleteLedgerAccountingRouteCache(ctx, route)
	if legacyErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete ledger-scoped transaction route cache", legacyErr)
	}

	return errors.Join(organizationErr, legacyErr)
}
