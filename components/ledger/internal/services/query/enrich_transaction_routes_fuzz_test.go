// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

// FuzzEnrichTransactionRoutes_SliceSize exercises the enrichment function with
// varying numbers of transaction routes (0 to N), varying junction mappings, and
// varying operation route results. The function must never panic regardless of
// input size or the shape of the mock-returned data.
//
// Fuzz inputs:
//   - numRoutes: number of transaction routes to create (bounded to 200)
//   - junctionMissRate: percentage (0-100) of routes that have NO junction entry
//   - opRouteMissRate: percentage (0-100) of junction entries whose operation
//     route is missing from the FindByIDs result (simulates partial fetch)
//   - junctionError: if true, the junction query returns an error
//   - findByIDsError: if true, the FindByIDs batch query returns an error
func FuzzEnrichTransactionRoutes_SliceSize(f *testing.F) {
	// Seed 1: empty slice (boundary — zero routes)
	f.Add(uint8(0), uint8(0), uint8(0), false, false)

	// Seed 2: single route, all linked, no errors (valid — minimal happy path)
	f.Add(uint8(1), uint8(0), uint8(0), false, false)

	// Seed 3: many routes, all linked, no errors (boundary — large input)
	f.Add(uint8(200), uint8(0), uint8(0), false, false)

	// Seed 4: many routes, 100% junction miss (empty junction map)
	f.Add(uint8(50), uint8(100), uint8(0), false, false)

	// Seed 5: many routes with partial opRoute misses (data inconsistency)
	f.Add(uint8(30), uint8(20), uint8(50), false, false)

	// Seed 6: junction query error path
	f.Add(uint8(10), uint8(0), uint8(0), true, false)

	// Seed 7: FindByIDs error path
	f.Add(uint8(10), uint8(0), uint8(0), false, true)

	// Seed 8: single route, junction error (security — error on smallest input)
	f.Add(uint8(1), uint8(0), uint8(0), true, false)

	// Seed 9: maximum miss rates (boundary — worst case data quality)
	f.Add(uint8(100), uint8(100), uint8(100), false, false)

	f.Fuzz(func(t *testing.T, numRoutes uint8, junctionMissRate uint8, opRouteMissRate uint8, junctionError bool, findByIDsError bool) {
		// Input bounding: cap route count to prevent excessive allocations
		if numRoutes > 200 {
			numRoutes = 200
		}

		// Normalize miss rates to 0-100
		jmr := junctionMissRate % 101
		omr := opRouteMissRate % 101

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTRRepo := transactionroute.NewMockRepository(ctrl)
		mockORRepo := operationroute.NewMockRepository(ctrl)

		uc := &UseCase{
			TransactionRouteRepo: mockTRRepo,
			OperationRouteRepo:   mockORRepo,
		}

		orgID := uuid.New()
		ledgerID := uuid.New()

		// Build the input slice of transaction routes
		routes := make([]*mmodel.TransactionRoute, int(numRoutes))
		for i := range routes {
			routes[i] = &mmodel.TransactionRoute{
				ID:             uuid.New(),
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "fuzz-route",
			}
		}

		// If empty slice, enrichment returns early — no mock expectations needed
		if numRoutes == 0 {
			err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes)
			if err != nil {
				t.Errorf("expected nil error for empty slice, got: %v", err)
			}

			return
		}

		// Build junction map based on miss rate
		junctionMap := make(map[uuid.UUID][]uuid.UUID)
		var allORIDs []uuid.UUID

		for i, tr := range routes {
			// Deterministic miss: routes at indices < (jmr% of total) are missed
			if uint8(i*100/int(numRoutes)) < jmr {
				continue // Not in junction map — simulates no links
			}

			orID := uuid.New()
			junctionMap[tr.ID] = []uuid.UUID{orID}
			allORIDs = append(allORIDs, orID)
		}

		if junctionError {
			mockTRRepo.EXPECT().
				FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("fuzz junction error"))

			err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes)
			if err == nil {
				t.Error("expected error from junction query, got nil")
			}

			return
		}

		mockTRRepo.EXPECT().
			FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
			Return(junctionMap, nil)

		// Build operation routes result based on opRoute miss rate
		if len(allORIDs) > 0 {
			var opRoutes []*mmodel.OperationRoute

			for i, orID := range allORIDs {
				// Deterministic miss: entries at indices < (omr% of total) are omitted
				if len(allORIDs) > 0 && uint8(i*100/len(allORIDs)) < omr {
					continue // Missing from FindByIDs result — simulates partial fetch
				}

				opRoutes = append(opRoutes, &mmodel.OperationRoute{
					ID:             orID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					Title:          "fuzz-op",
					OperationType:  "source",
				})
			}

			if findByIDsError {
				mockORRepo.EXPECT().
					FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
					Return(nil, errors.New("fuzz findByIDs error"))

				err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes)
				if err == nil {
					t.Error("expected error from FindByIDs, got nil")
				}

				return
			}

			mockORRepo.EXPECT().
				FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
				Return(opRoutes, nil)
		}

		// Primary property: the function must NEVER panic
		err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes)

		// Secondary property: on success, every route must have a non-nil OperationRoutes slice
		if err == nil {
			for i, tr := range routes {
				if tr.OperationRoutes == nil {
					t.Errorf("route[%d] (%s) has nil OperationRoutes after successful enrichment", i, tr.ID)
				}
			}
		}
	})
}

// FuzzEnrichTransactionRoutes_JunctionMapShape exercises the enrichment function
// with varying junction map shapes: routes pointing to many operation routes,
// duplicate operation route IDs across transaction routes, and empty slices in
// the junction map. The function must never panic and must always produce
// non-nil OperationRoutes slices on success.
//
// Fuzz inputs:
//   - numRoutes: number of transaction routes (bounded to 50)
//   - opsPerRoute: number of operation route IDs per junction entry (bounded to 20)
//   - sharedOps: whether all routes share the same set of operation route IDs
//   - includeEmptySlices: whether some junction entries have empty []uuid.UUID
func FuzzEnrichTransactionRoutes_JunctionMapShape(f *testing.F) {
	// Seed 1: single route, single op (minimal valid case)
	f.Add(uint8(1), uint8(1), false, false)

	// Seed 2: many routes, many ops, no sharing (wide junction map)
	f.Add(uint8(50), uint8(20), false, false)

	// Seed 3: many routes sharing same ops (deduplication stress test)
	f.Add(uint8(30), uint8(5), true, false)

	// Seed 4: routes with empty junction slices mixed in
	f.Add(uint8(20), uint8(3), false, true)

	// Seed 5: single route, max ops per route (boundary)
	f.Add(uint8(1), uint8(20), false, false)

	// Seed 6: zero routes (boundary — empty slice)
	f.Add(uint8(0), uint8(5), false, false)

	// Seed 7: all routes share ops AND have empty slices mixed in
	f.Add(uint8(15), uint8(10), true, true)

	f.Fuzz(func(t *testing.T, numRoutes uint8, opsPerRoute uint8, sharedOps bool, includeEmptySlices bool) {
		// Input bounding
		if numRoutes > 50 {
			numRoutes = 50
		}

		if opsPerRoute > 20 {
			opsPerRoute = 20
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTRRepo := transactionroute.NewMockRepository(ctrl)
		mockORRepo := operationroute.NewMockRepository(ctrl)

		uc := &UseCase{
			TransactionRouteRepo: mockTRRepo,
			OperationRouteRepo:   mockORRepo,
		}

		orgID := uuid.New()
		ledgerID := uuid.New()

		routes := make([]*mmodel.TransactionRoute, int(numRoutes))
		for i := range routes {
			routes[i] = &mmodel.TransactionRoute{
				ID:             uuid.New(),
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          "fuzz-shape-route",
			}
		}

		if numRoutes == 0 {
			err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes)
			if err != nil {
				t.Errorf("expected nil error for empty slice, got: %v", err)
			}

			return
		}

		// Build a shared pool of operation route IDs (used when sharedOps=true)
		sharedORIDs := make([]uuid.UUID, int(opsPerRoute))
		for i := range sharedORIDs {
			sharedORIDs[i] = uuid.New()
		}

		// Build junction map
		junctionMap := make(map[uuid.UUID][]uuid.UUID)
		uniqueORIDs := make(map[uuid.UUID]struct{})

		for i, tr := range routes {
			// Mix in empty slices for even-indexed routes when includeEmptySlices=true
			if includeEmptySlices && i%2 == 0 {
				junctionMap[tr.ID] = []uuid.UUID{}

				continue
			}

			var orIDs []uuid.UUID
			if sharedOps {
				orIDs = make([]uuid.UUID, len(sharedORIDs))
				copy(orIDs, sharedORIDs)
			} else {
				orIDs = make([]uuid.UUID, int(opsPerRoute))
				for j := range orIDs {
					orIDs[j] = uuid.New()
				}
			}

			junctionMap[tr.ID] = orIDs

			for _, id := range orIDs {
				uniqueORIDs[id] = struct{}{}
			}
		}

		mockTRRepo.EXPECT().
			FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
			Return(junctionMap, nil)

		// Build the full set of operation routes matching all unique IDs
		if len(uniqueORIDs) > 0 {
			var opRoutes []*mmodel.OperationRoute
			for orID := range uniqueORIDs {
				opRoutes = append(opRoutes, &mmodel.OperationRoute{
					ID:             orID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					Title:          "fuzz-shape-op",
					OperationType:  "source",
				})
			}

			mockORRepo.EXPECT().
				FindByIDs(gomock.Any(), orgID, ledgerID, gomock.Any()).
				Return(opRoutes, nil)
		}

		// Primary property: no panic
		err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), routes)

		// Secondary properties on success:
		if err == nil {
			for i, tr := range routes {
				// Non-nil OperationRoutes
				if tr.OperationRoutes == nil {
					t.Errorf("route[%d] (%s) has nil OperationRoutes after successful enrichment", i, tr.ID)
				}

				// Routes with empty junction entries should have 0 operation routes
				jOrIDs := junctionMap[tr.ID]
				if len(jOrIDs) == 0 && len(tr.OperationRoutes) != 0 {
					t.Errorf("route[%d] (%s) expected 0 operation routes (empty junction), got %d",
						i, tr.ID, len(tr.OperationRoutes))
				}

				// Routes with non-empty junction entries should have <= len(jOrIDs) operation routes
				// (could be less if FindByIDs returned fewer, but never more)
				if len(jOrIDs) > 0 && len(tr.OperationRoutes) > len(jOrIDs) {
					t.Errorf("route[%d] (%s) has %d operation routes but junction only has %d entries",
						i, tr.ID, len(tr.OperationRoutes), len(jOrIDs))
				}
			}
		}
	})
}
