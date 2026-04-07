// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// --- helpers -----------------------------------------------------------------

// buildEnrichmentScenario constructs a deterministic test scenario from random seeds.
// It builds N transaction routes (bounded to [0, maxRoutes]), a junction map where
// each route maps to [0, maxOpsPerRoute] operation route IDs, and a full set of
// operation route objects for all IDs in the junction map. This helper isolates
// scenario construction from property assertions.
type enrichmentScenario struct {
	routes      []*mmodel.TransactionRoute
	junctionMap map[uuid.UUID][]uuid.UUID
	opRoutes    []*mmodel.OperationRoute
	orgID       uuid.UUID
	ledgerID    uuid.UUID
}

func buildEnrichmentScenario(numRoutes, maxOpsPerRoute uint8) enrichmentScenario {
	const maxRoutes = 50

	n := int(numRoutes % (maxRoutes + 1))
	opsPerRoute := int(maxOpsPerRoute%10) + 1

	orgID := uuid.New()
	ledgerID := uuid.New()

	routes := make([]*mmodel.TransactionRoute, n)
	for i := range routes {
		routes[i] = &mmodel.TransactionRoute{
			ID:             uuid.New(),
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "prop-route",
		}
	}

	junctionMap := make(map[uuid.UUID][]uuid.UUID)
	uniqueORIDs := make(map[uuid.UUID]struct{})

	for _, tr := range routes {
		orIDs := make([]uuid.UUID, opsPerRoute)
		for j := range orIDs {
			orIDs[j] = uuid.New()
			uniqueORIDs[orIDs[j]] = struct{}{}
		}

		junctionMap[tr.ID] = orIDs
	}

	var opRoutes []*mmodel.OperationRoute
	for orID := range uniqueORIDs {
		opRoutes = append(opRoutes, &mmodel.OperationRoute{
			ID:             orID,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			Title:          "prop-op",
			OperationType:  "source",
		})
	}

	return enrichmentScenario{
		routes:      routes,
		junctionMap: junctionMap,
		opRoutes:    opRoutes,
		orgID:       orgID,
		ledgerID:    ledgerID,
	}
}

// setupMocksForScenario wires up gomock expectations for the standard happy-path
// enrichment flow: junction query returns the scenario's junctionMap, FindByIDs
// returns the scenario's opRoutes.
func setupMocksForScenario(
	ctrl *gomock.Controller,
	s enrichmentScenario,
) *UseCase {
	mockTRRepo := transactionroute.NewMockRepository(ctrl)
	mockORRepo := operationroute.NewMockRepository(ctrl)

	if len(s.routes) > 0 {
		mockTRRepo.EXPECT().
			FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
			Return(s.junctionMap, nil)

		if len(s.opRoutes) > 0 {
			mockORRepo.EXPECT().
				FindByIDs(gomock.Any(), s.orgID, s.ledgerID, gomock.Any()).
				Return(s.opRoutes, nil)
		}
	}

	return &UseCase{
		TransactionRouteRepo: mockTRRepo,
		OperationRouteRepo:   mockORRepo,
	}
}

// --- property tests ----------------------------------------------------------

// TestProperty_EnrichTransactionRoutes_Conservation verifies that enrichment never
// adds or removes transaction routes. The output slice length must always equal the
// input slice length, regardless of junction map contents.
//
// PROPERTY: len(routes_before) == len(routes_after)
func TestProperty_EnrichTransactionRoutes_Conservation(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	property := func(numRoutes, maxOpsPerRoute uint8) bool {
		s := buildEnrichmentScenario(numRoutes, maxOpsPerRoute)
		countBefore := len(s.routes)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := setupMocksForScenario(ctrl, s)

		err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), s.routes)
		if err != nil {
			return false
		}

		// PROPERTY: enrichment preserves the number of transaction routes
		return len(s.routes) == countBefore
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}

// TestProperty_EnrichTransactionRoutes_NoNilOperationRoutes verifies that after
// successful enrichment, every transaction route has a non-nil OperationRoutes slice.
// Routes without junction entries get an empty slice ([]OperationRoute{}), never nil.
//
// PROPERTY: ∀ tr ∈ routes: tr.OperationRoutes != nil
func TestProperty_EnrichTransactionRoutes_NoNilOperationRoutes(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	property := func(numRoutes, maxOpsPerRoute uint8) bool {
		s := buildEnrichmentScenario(numRoutes, maxOpsPerRoute)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := setupMocksForScenario(ctrl, s)

		err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), s.routes)
		if err != nil {
			return false
		}

		// PROPERTY: every route has non-nil OperationRoutes after enrichment
		for _, tr := range s.routes {
			if tr.OperationRoutes == nil {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}

// TestProperty_EnrichTransactionRoutes_CorrectAssignment verifies that every
// operation route assigned to a transaction route was present in the junction
// mapping for that specific transaction route ID. No cross-contamination occurs.
//
// PROPERTY: ∀ tr: ∀ or ∈ tr.OperationRoutes: or.ID ∈ junctionMap[tr.ID]
func TestProperty_EnrichTransactionRoutes_CorrectAssignment(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	property := func(numRoutes, maxOpsPerRoute uint8) bool {
		s := buildEnrichmentScenario(numRoutes, maxOpsPerRoute)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := setupMocksForScenario(ctrl, s)

		err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), s.routes)
		if err != nil {
			return false
		}

		// Build a quick lookup: junctionMap[trID] → set of allowed orIDs
		allowedByTR := make(map[uuid.UUID]map[uuid.UUID]struct{})
		for trID, orIDs := range s.junctionMap {
			set := make(map[uuid.UUID]struct{}, len(orIDs))
			for _, orID := range orIDs {
				set[orID] = struct{}{}
			}

			allowedByTR[trID] = set
		}

		// PROPERTY: every assigned operation route ID is in the junction set for its parent
		for _, tr := range s.routes {
			allowed := allowedByTR[tr.ID]
			for _, or := range tr.OperationRoutes {
				if _, ok := allowed[or.ID]; !ok {
					return false
				}
			}
		}

		return true
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}

// TestProperty_EnrichTransactionRoutes_NoOrphanOperationRoutes verifies that every
// operation route returned from the batch FindByIDs ends up assigned to at least
// one transaction route. No fetched operation route is wasted/orphaned.
//
// PROPERTY: ∀ or ∈ fetchedOpRoutes: ∃ tr: or.ID ∈ tr.OperationRoutes
func TestProperty_EnrichTransactionRoutes_NoOrphanOperationRoutes(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	property := func(numRoutes, maxOpsPerRoute uint8) bool {
		s := buildEnrichmentScenario(numRoutes, maxOpsPerRoute)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := setupMocksForScenario(ctrl, s)

		err := uc.enrichTransactionRoutesWithOperationRoutes(context.Background(), s.routes)
		if err != nil {
			return false
		}

		if len(s.routes) == 0 {
			return true // No routes → no operation routes → nothing to check
		}

		// Collect all assigned operation route IDs across all transaction routes
		assignedIDs := make(map[uuid.UUID]struct{})
		for _, tr := range s.routes {
			for _, or := range tr.OperationRoutes {
				assignedIDs[or.ID] = struct{}{}
			}
		}

		// PROPERTY: every fetched operation route must appear in at least one assignment
		for _, or := range s.opRoutes {
			if _, assigned := assignedIDs[or.ID]; !assigned {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}

// TestProperty_EnrichTransactionRoutes_Idempotency verifies that running enrichment
// twice on the same data produces the same result. After the first enrichment the
// OperationRoutes field is populated; a second enrichment (with the same junction
// data) must produce identical assignments.
//
// PROPERTY: enrich(enrich(routes)) == enrich(routes)
func TestProperty_EnrichTransactionRoutes_Idempotency(t *testing.T) {
	t.Parallel()

	cfg := &quick.Config{MaxCount: 100}

	property := func(numRoutes, maxOpsPerRoute uint8) bool {
		s := buildEnrichmentScenario(numRoutes, maxOpsPerRoute)

		// --- First enrichment ---
		ctrl1 := gomock.NewController(t)
		uc1 := setupMocksForScenario(ctrl1, s)

		err := uc1.enrichTransactionRoutesWithOperationRoutes(context.Background(), s.routes)
		ctrl1.Finish()

		if err != nil {
			return false
		}

		// Snapshot the first enrichment result
		firstPass := make(map[uuid.UUID][]uuid.UUID, len(s.routes))
		for _, tr := range s.routes {
			ids := make([]uuid.UUID, len(tr.OperationRoutes))
			for i, or := range tr.OperationRoutes {
				ids[i] = or.ID
			}

			firstPass[tr.ID] = ids
		}

		// --- Second enrichment (same junction data, same opRoutes) ---
		ctrl2 := gomock.NewController(t)
		uc2 := setupMocksForScenario(ctrl2, s)

		err = uc2.enrichTransactionRoutesWithOperationRoutes(context.Background(), s.routes)
		ctrl2.Finish()

		if err != nil {
			return false
		}

		// Snapshot the second enrichment result
		secondPass := make(map[uuid.UUID][]uuid.UUID, len(s.routes))
		for _, tr := range s.routes {
			ids := make([]uuid.UUID, len(tr.OperationRoutes))
			for i, or := range tr.OperationRoutes {
				ids[i] = or.ID
			}

			secondPass[tr.ID] = ids
		}

		// PROPERTY: first and second enrichment produce identical operation route ID sets
		for trID, firstIDs := range firstPass {
			secondIDs, exists := secondPass[trID]
			if !exists {
				return false
			}

			if len(firstIDs) != len(secondIDs) {
				return false
			}

			// Compare as sets (order may vary due to map iteration in production code)
			firstSet := make(map[uuid.UUID]struct{}, len(firstIDs))
			for _, id := range firstIDs {
				firstSet[id] = struct{}{}
			}

			for _, id := range secondIDs {
				if _, ok := firstSet[id]; !ok {
					return false
				}
			}
		}

		return true
	}

	err := quick.Check(property, cfg)
	require.NoError(t, err)
}
