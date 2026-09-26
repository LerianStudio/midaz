// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package command

import (
	"context"
	"encoding/json"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// Route validation and rubric stamping read the cached transaction route, which
// carries each operation route's code and accounting entries and never expires.
// Patching any of them must refresh the transaction routes that link the
// operation route, or transactions keep validating against the old rules.
func TestIntegration_UpdateOperationRoute_RefreshesTheCacheOfLinkedTransactionRoutes(t *testing.T) {
	tests := []struct {
		name   string
		input  func(t *testing.T) *mmodel.UpdateOperationRouteInput
		assert func(t *testing.T, route mmodel.OperationRouteCache)
	}{
		{
			name: "a new rubric",
			input: func(t *testing.T) *mmodel.UpdateOperationRouteInput {
				t.Helper()

				entries := &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{Debit: &mmodel.AccountingRubric{Code: "PATCHED-1000", Description: "Patched rubric"}}}
				raw, err := json.Marshal(entries)
				require.NoError(t, err)

				return &mmodel.UpdateOperationRouteInput{AccountingEntries: entries, AccountingEntriesRaw: raw}
			},
			assert: func(t *testing.T, route mmodel.OperationRouteCache) {
				t.Helper()

				require.NotNil(t, route.AccountingEntries)
				require.NotNil(t, route.AccountingEntries.Direct)
				assert.Equal(t, "PATCHED-1000", route.AccountingEntries.Direct.Debit.Code)
			},
		},
		{
			name: "a new code",
			input: func(*testing.T) *mmodel.UpdateOperationRouteInput {
				return &mmodel.UpdateOperationRouteInput{Code: "PATCHED-CODE"}
			},
			assert: func(t *testing.T, route mmodel.OperationRouteCache) {
				t.Helper()

				assert.Equal(t, "PATCHED-CODE", route.Code)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			infra := setupReloadCacheTestInfra(t)
			db := infra.pgContainer.DB
			ctx := context.Background()

			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

			sourceID := createRoutedOperationRoute(t, db, orgID, ledgerID, "source", "debit")
			destinationID := createRoutedOperationRoute(t, db, orgID, ledgerID, "destination", "credit")
			txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, db, orgID, ledgerID, "Operation route patch")
			pgtestutil.CreateTestOperationTransactionRouteLink(t, db, sourceID, txRouteID)
			pgtestutil.CreateTestOperationTransactionRouteLink(t, db, destinationID, txRouteID)

			reader := &query.UseCase{TransactionRouteRepo: infra.uc.TransactionRouteRepo, TransactionRedisRepo: infra.uc.TransactionRedisRepo}

			_, err := reader.GetOrCreateTransactionRouteCache(ctx, orgID, txRouteID)
			require.NoError(t, err, "the transaction route is cached before the patch")

			metadataRepo := mongodb.NewMockRepository(gomock.NewController(t))
			metadataRepo.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			metadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			infra.uc.TransactionMetadataRepo = metadataRepo

			_, err = infra.uc.UpdateOperationRoute(ctx, orgID, sourceID, tc.input(t))
			require.NoError(t, err)

			cached, err := reader.GetOrCreateTransactionRouteCache(ctx, orgID, txRouteID)
			require.NoError(t, err)

			route, ok := cached.Actions["direct"].Source[sourceID.String()]
			require.True(t, ok, "the patched route stays in the direct action")
			tc.assert(t, route)
		})
	}
}
