// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// Route cache entries never expire, and pods of the previous version read a key
// scoped by the ledger the route was created under. These tests seed that key the
// way such a pod writes it and prove every write path of the current version
// removes it, so an old pod reloads the route from the database instead of
// serving a rule that has since changed or been deleted.

func TestIntegration_TransactionRouteCache_UpdateDropsLedgerKeyAndRewritesOrganizationKey(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)
	db := infra.pgContainer.DB
	ctx := context.Background()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerA := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerB := uuid.Must(libCommons.GenerateUUIDv7())

	sourceID := createRoutedOperationRoute(t, db, orgID, ledgerA, "source", "debit")
	oldDestinationID := createRoutedOperationRoute(t, db, orgID, ledgerA, "destination", "credit")
	newDestinationID := createRoutedOperationRoute(t, db, orgID, ledgerB, "destination", "credit")

	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, db, orgID, ledgerA, "Update transition route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, sourceID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, oldDestinationID, txRouteID)

	legacyKey := utils.LedgerAccountingRoutesInternalKey(orgID, ledgerA, txRouteID)
	organizationKey := utils.AccountingRoutesInternalKey(orgID, txRouteID)

	current, err := infra.uc.TransactionRouteRepo.FindByID(ctx, orgID, txRouteID)
	require.NoError(t, err)
	seedRouteCache(t, ctx, infra.uc, legacyKey, current)
	seedRouteCache(t, ctx, infra.uc, organizationKey, current)

	metadataRepo := mongodb.NewMockRepository(gomock.NewController(t))
	metadataRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	infra.uc.TransactionMetadataRepo = metadataRepo

	operationRoutes := []uuid.UUID{sourceID, newDestinationID}
	updated, err := infra.uc.UpdateTransactionRoute(ctx, orgID, txRouteID, &mmodel.UpdateTransactionRouteInput{OperationRoutes: &operationRoutes})
	require.NoError(t, err)
	require.NoError(t, infra.uc.CreateAccountingRouteCache(ctx, updated))

	assertRouteCacheAbsent(t, ctx, infra.uc, legacyKey)

	cached := requireRouteCache(t, ctx, infra.uc, organizationKey)
	direct := cached.Actions["direct"]
	assert.Contains(t, direct.Destination, newDestinationID.String(), "the organization key must hold the rules after the update")
	assert.NotContains(t, direct.Destination, oldDestinationID.String(), "the unlinked destination must not survive in the organization key")
}

func TestIntegration_TransactionRouteCache_OperationRouteReloadDropsLedgerKeysOfEveryLinkedRoute(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)
	db := infra.pgContainer.DB
	ctx := context.Background()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerA := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerB := uuid.Must(libCommons.GenerateUUIDv7())

	sharedSourceID := createRoutedOperationRoute(t, db, orgID, ledgerA, "source", "debit")
	destinationA := createRoutedOperationRoute(t, db, orgID, ledgerA, "destination", "credit")
	destinationB := createRoutedOperationRoute(t, db, orgID, ledgerB, "destination", "credit")

	txRouteA := pgtestutil.CreateTestTransactionRouteSimple(t, db, orgID, ledgerA, "Route under ledger A")
	txRouteB := pgtestutil.CreateTestTransactionRouteSimple(t, db, orgID, ledgerB, "Route under ledger B")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, sharedSourceID, txRouteA)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, destinationA, txRouteA)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, sharedSourceID, txRouteB)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, destinationB, txRouteB)

	staleA := &mmodel.TransactionRoute{ID: txRouteA, OrganizationID: orgID}
	staleB := &mmodel.TransactionRoute{ID: txRouteB, OrganizationID: orgID}
	legacyA := utils.LedgerAccountingRoutesInternalKey(orgID, ledgerA, txRouteA)
	legacyB := utils.LedgerAccountingRoutesInternalKey(orgID, ledgerB, txRouteB)
	seedRouteCache(t, ctx, infra.uc, legacyA, staleA)
	seedRouteCache(t, ctx, infra.uc, legacyB, staleB)
	seedRouteCache(t, ctx, infra.uc, utils.AccountingRoutesInternalKey(orgID, txRouteA), staleA)
	seedRouteCache(t, ctx, infra.uc, utils.AccountingRoutesInternalKey(orgID, txRouteB), staleB)

	require.NoError(t, infra.uc.ReloadOperationRouteCache(ctx, orgID, sharedSourceID))

	assertRouteCacheAbsent(t, ctx, infra.uc, legacyA)
	assertRouteCacheAbsent(t, ctx, infra.uc, legacyB)

	for _, txRouteID := range []uuid.UUID{txRouteA, txRouteB} {
		cached := requireRouteCache(t, ctx, infra.uc, utils.AccountingRoutesInternalKey(orgID, txRouteID))
		assert.Contains(t, cached.Actions["direct"].Source, sharedSourceID.String(),
			"transaction route %s must be refreshed under its organization key", txRouteID)
	}
}

func TestIntegration_TransactionRouteCache_DeleteDropsBothKeysAndTheRouteIsNotFound(t *testing.T) {
	infra := setupReloadCacheTestInfra(t)
	db := infra.pgContainer.DB
	ctx := context.Background()

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerA := uuid.Must(libCommons.GenerateUUIDv7())

	sourceID := createRoutedOperationRoute(t, db, orgID, ledgerA, "source", "debit")
	destinationID := createRoutedOperationRoute(t, db, orgID, ledgerA, "destination", "credit")
	txRouteID := pgtestutil.CreateTestTransactionRouteSimple(t, db, orgID, ledgerA, "Delete transition route")
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, sourceID, txRouteID)
	pgtestutil.CreateTestOperationTransactionRouteLink(t, db, destinationID, txRouteID)

	current, err := infra.uc.TransactionRouteRepo.FindByID(ctx, orgID, txRouteID)
	require.NoError(t, err)

	legacyKey := utils.LedgerAccountingRoutesInternalKey(orgID, ledgerA, txRouteID)
	organizationKey := utils.AccountingRoutesInternalKey(orgID, txRouteID)
	seedRouteCache(t, ctx, infra.uc, legacyKey, current)
	require.NoError(t, infra.uc.CreateAccountingRouteCache(ctx, current))
	requireRouteCache(t, ctx, infra.uc, organizationKey)

	require.NoError(t, infra.uc.DeleteTransactionRouteByID(ctx, orgID, txRouteID))

	assertRouteCacheAbsent(t, ctx, infra.uc, legacyKey)
	assertRouteCacheAbsent(t, ctx, infra.uc, organizationKey)

	reader := &query.UseCase{TransactionRouteRepo: infra.uc.TransactionRouteRepo, TransactionRedisRepo: infra.uc.TransactionRedisRepo}

	_, err = reader.GetOrCreateTransactionRouteCache(ctx, orgID, txRouteID)

	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound, "a deleted route must not be served from the cache")
	assert.Equal(t, "0105", notFound.Code)
}

// createRoutedOperationRoute creates an operation route with a direct accounting
// entry, so it appears in the "direct" action of every cache built from it.
func createRoutedOperationRoute(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, operationType, direction string) uuid.UUID {
	t.Helper()

	id := pgtestutil.CreateTestOperationRouteSimple(t, db, orgID, ledgerID, operationType+" "+direction, operationType)
	entries := fmt.Sprintf(`{"direct":{"%s":{"code":"%s","description":"%s"}}}`, direction, id.String(), operationType)

	_, err := db.Exec(`UPDATE operation_route SET accounting_entries=$1::jsonb WHERE id=$2`, entries, id)
	require.NoError(t, err, "seed operation route accounting entries")

	return id
}

// seedRouteCache writes route's cache at key with no expiry, as any writer does.
func seedRouteCache(t *testing.T, ctx context.Context, uc *UseCase, key string, route *mmodel.TransactionRoute) {
	t.Helper()

	cacheBytes, err := route.ToCache().ToMsgpack()
	require.NoError(t, err)
	require.NoError(t, uc.TransactionRedisRepo.SetBytes(ctx, key, cacheBytes, 0))
}

func requireRouteCache(t *testing.T, ctx context.Context, uc *UseCase, key string) mmodel.TransactionRouteCache {
	t.Helper()

	cachedBytes, err := uc.TransactionRedisRepo.GetBytes(ctx, key)
	require.NoError(t, err, "expected a cache entry at %s", key)

	var cached mmodel.TransactionRouteCache
	require.NoError(t, cached.FromMsgpack(cachedBytes))

	return cached
}

func assertRouteCacheAbsent(t *testing.T, ctx context.Context, uc *UseCase, key string) {
	t.Helper()

	_, err := uc.TransactionRedisRepo.GetBytes(ctx, key)
	assert.Truef(t, errors.Is(err, goredis.Nil), "expected no cache entry at %s, got err=%v", key, err)
}
