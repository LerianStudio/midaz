// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// enrichTransactionRoutesWithOperationRoutes performs a post-query batch enrichment
// of transaction routes with their linked OperationRoute objects.
// It executes at most 2 additional queries (junction + batch FindByIDs) regardless of
// result size, avoiding N+1 query problems.
//
// Each transaction route in the slice will have its OperationRoutes field set:
//   - populated []OperationRoute when links exist
//   - empty []OperationRoute{} (not nil) when no links exist
func (uc *UseCase) enrichTransactionRoutesWithOperationRoutes(ctx context.Context, transactionRoutes []*mmodel.TransactionRoute) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.enrich_transaction_routes_with_operation_routes")
	defer span.End()

	if len(transactionRoutes) == 0 {
		return nil
	}

	// Step 1: Collect all transaction route IDs
	trIDs := make([]uuid.UUID, len(transactionRoutes))
	for i, tr := range transactionRoutes {
		trIDs[i] = tr.ID
	}

	// Step 2: Batch query junction table
	junctionMap, err := uc.TransactionRouteRepo.FindOperationRouteIDsByTransactionRouteIDs(ctx, trIDs)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to fetch operation route IDs from junction table", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to fetch operation route IDs from junction table: %v", err))

		return err
	}

	// Step 3: Collect all unique operation route IDs
	uniqueORIDs := make(map[uuid.UUID]struct{})

	for _, orIDs := range junctionMap {
		for _, orID := range orIDs {
			uniqueORIDs[orID] = struct{}{}
		}
	}

	// Step 4: Batch fetch operation route objects (only if there are any)
	orMap := make(map[uuid.UUID]*mmodel.OperationRoute)

	if len(uniqueORIDs) > 0 {
		orIDSlice := make([]uuid.UUID, 0, len(uniqueORIDs))
		for orID := range uniqueORIDs {
			orIDSlice = append(orIDSlice, orID)
		}

		// Use the first transaction route's org/ledger IDs (all share the same scope)
		orgID := transactionRoutes[0].OrganizationID
		ledgerID := transactionRoutes[0].LedgerID

		opRoutes, err := uc.OperationRouteRepo.FindByIDs(ctx, orgID, ledgerID, orIDSlice)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to batch fetch operation routes", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to batch fetch operation routes: %v", err))

			return err
		}

		for _, or := range opRoutes {
			orMap[or.ID] = or
		}
	}

	// Step 5: Assign operation routes to their parent transaction routes
	for _, tr := range transactionRoutes {
		orIDs, exists := junctionMap[tr.ID]
		if !exists || len(orIDs) == 0 {
			tr.OperationRoutes = make([]mmodel.OperationRoute, 0)

			continue
		}

		routes := make([]mmodel.OperationRoute, 0, len(orIDs))

		for _, orID := range orIDs {
			if or, ok := orMap[orID]; ok {
				routes = append(routes, *or)
			}
		}

		tr.OperationRoutes = routes
	}

	logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Enriched %d transaction routes with operation routes", len(transactionRoutes)))

	return nil
}
