// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// =============================================================================
// BALANCE DELETE CACHE-COHERENCE INTEGRATION TESTS (honored-lock)
// =============================================================================
// These tests lock the fixed behavior of both balance delete paths against the
// cache-coherence regression where a deleted balance's Redis cache entry
// survived the delete, so a later transaction kept operating on the removed
// balance ("phantom" activity on a deleted account).
//
// With the fix, a delete dual-writes a long-lived dedicated marker and the legacy
// "<balanceKey>:deleted" marker for the rolling-deploy window, soft-deletes the
// PostgreSQL row, evicts (Del) the balance cache key, and shortens both owned markers.
// A later transaction can no longer find the balance (cache evicted + row
// soft-deleted, which the PG query filters out), so it is rejected with
// ErrAccountIneligibility (0019 / HTTP 422) instead of succeeding.
//
// Pre-fix, the crediting transaction in each case would have SUCCEEDED (HTTP
// 201) and mutated the deleted balance, because its cache entry lingered with
// AllowReceiving=1. That success-vs-rejection flip is the regression these
// tests guard.
//
// Delete is exercised at the use-case level (no HTTP delete route is wired into
// this harness); the transaction flow runs through the real Fiber HTTP handler.
// Both sides run against real Postgres + Redis + the Lua atomic script.

// balanceCacheDeleteAsset is the shared asset code for these scenarios. It must
// match on both sides of every transaction so asset validation passes.
const balanceCacheDeleteAsset = "BRL"

// postTransactionJSON posts a single JSON transfer of value from sourceAlias to
// destAlias through the real HTTP handler and returns the HTTP status code and
// the raw response body.
func postTransactionJSON(t *testing.T, infra *testInfra, sourceAlias, destAlias, asset, value string) (int, string) {
	t.Helper()

	body := fmt.Sprintf(`{
		"description": "cache-coherence integration transfer",
		"pending": false,
		"send": {
			"asset": %[1]q,
			"value": %[2]q,
			"source": {
				"from": [
					{
						"accountAlias": %[3]q,
						"amount": { "asset": %[1]q, "value": %[2]q }
					}
				]
			},
			"distribute": {
				"to": [
					{
						"accountAlias": %[4]q,
						"amount": { "asset": %[1]q, "value": %[2]q }
					}
				]
			}
		}
	}`, asset, value, sourceAlias, destAlias)

	req := httptest.NewRequest(http.MethodPost,
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/json",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "HTTP request should not fail")

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read response body")

	return resp.StatusCode, string(raw)
}

// wireEmptyLedgerSettings satisfies the transaction-create flow's ledger
// settings lookup, which this harness otherwise leaves unwired. Empty settings
// mean route and account-type validation default off, so a plain transfer flows
// through the balance path being exercised here.
func wireEmptyLedgerSettings(t *testing.T, infra *testInfra) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockLedgerRepo.EXPECT().
		GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{}, nil).
		AnyTimes()

	infra.handler.Query.LedgerRepo = mockLedgerRepo
}

// deleteMarkerCacheKey returns the delete marker key Redis holds for a balance while a
// delete is armed. It mirrors the production marker namespace while leaving tenant
// namespacing to the Redis repository wrapper.
func deleteMarkerCacheKey(orgID, ledgerID uuid.UUID, alias, key string) string {
	const (
		balanceCacheNamespacePrefix = "balance:{transactions}:"
		deleteMarkerNamespacePrefix = "balance_delete_marker:{transactions}:"
	)

	balanceKey := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+key)

	return deleteMarkerNamespacePrefix + strings.TrimPrefix(balanceKey, balanceCacheNamespacePrefix)
}

// TestIntegration_BalanceDeleteCacheEviction_AccountCascade drives the account
// cascade delete path (DeleteAllBalancesByAccountID): after a drained balance's
// account is deleted, its cache key is evicted, a delete marker is armed, and
// a subsequent crediting transaction is rejected with 0019 instead of mutating
// the removed balance.
func TestIntegration_BalanceDeleteCacheEviction_AccountCascade(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	wireEmptyLedgerSettings(t, infra)

	ctx := context.Background()

	deletedAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	counterpartyAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	const deletedAlias = "@cascade-deleted"
	const counterpartyAlias = "@cascade-funder"

	// The to-be-deleted balance is created already at zero available so it is
	// eligible for deletion (a funded balance cannot be deleted). The
	// counterparty carries the working funds for the round-trip below.
	deletedParams := postgrestestutil.DefaultBalanceParams()
	deletedParams.Alias = deletedAlias
	deletedParams.AssetCode = balanceCacheDeleteAsset
	deletedParams.Available = decimal.Zero
	deletedParams.OnHold = decimal.Zero
	postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, deletedAccountID, deletedParams)

	counterpartyParams := postgrestestutil.DefaultBalanceParams()
	counterpartyParams.Alias = counterpartyAlias
	counterpartyParams.AssetCode = balanceCacheDeleteAsset
	counterpartyParams.Available = decimal.NewFromInt(100)
	counterpartyParams.OnHold = decimal.Zero
	postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, counterpartyAccountID, counterpartyParams)

	// Round-trip: fund the balance from the counterparty, then send it all back.
	// This populates both Redis cache entries via the Lua atomic script and
	// leaves the to-be-deleted balance back at zero available (still deletable).
	status, respBody := postTransactionJSON(t, infra, counterpartyAlias, deletedAlias, balanceCacheDeleteAsset, "50")
	require.Equal(t, 201, status, "funding transaction should succeed: %s", respBody)

	status, respBody = postTransactionJSON(t, infra, deletedAlias, counterpartyAlias, balanceCacheDeleteAsset, "50")
	require.Equal(t, 201, status, "return transaction should succeed: %s", respBody)

	// The zeroed balance must be present in the cache before delete.
	cached := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	require.NotNil(t, cached, "balance cache key must exist before delete")
	require.True(t, cached.Available.Equal(decimal.Zero),
		"drained balance available should be zero, got %s", cached.Available.String())

	// Delete the whole account (cascade over all its balances).
	err := infra.handler.Command.DeleteAllBalancesByAccountID(ctx,
		infra.orgID, infra.ledgerID, deletedAccountID, "integration-cascade-delete-request")
	require.NoError(t, err, "account cascade delete should succeed")

	// (i) The balance cache key is evicted.
	evicted := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	assert.Nil(t, evicted, "balance cache key must be evicted after delete")

	// (ii) The honored-lock delete marker is armed in the dedicated namespace (and its
	// one-release legacy compatibility key).
	deleteMarker, err := infra.redisRepo.Get(ctx, deleteMarkerCacheKey(infra.orgID, infra.ledgerID, deletedAlias, "default"))
	require.NoError(t, err)
	assert.NotEmpty(t, deleteMarker, "delete marker must be present after delete")
	parsedToken, parseErr := uuid.Parse(deleteMarker)
	assert.NoError(t, parseErr, "delete marker must contain a valid UUID ownership token")
	assert.NotEqual(t, uuid.Nil, parsedToken, "delete marker ownership token must be non-nil")

	// (iii) A crediting transaction to the deleted balance is rejected with 0019.
	// Pre-fix this would have succeeded on the lingering cache entry.
	status, respBody = postTransactionJSON(t, infra, counterpartyAlias, deletedAlias, balanceCacheDeleteAsset, "100")
	assert.Equal(t, 422, status, "transaction on a deleted balance must be rejected, got %d: %s", status, respBody)
	assert.True(t, strings.Contains(respBody, cn.ErrAccountIneligibility.Error()),
		"rejection should carry 0019, got: %s", respBody)

	// The counterparty balance must not have been mutated by the rejected transaction.
	counterparty := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, counterpartyAlias, "default")
	require.NotNil(t, counterparty, "counterparty balance should still exist")
	assert.True(t, counterparty.Available.Equal(decimal.NewFromInt(100)),
		"counterparty available must be unchanged (100) after the rejected transaction, got %s", counterparty.Available.String())
}

// TestIntegration_BalanceDeleteCascadeRejectsRedisOnlyOverdraftDebt verifies the
// cascade path reads OverdraftUsed from the real Redis snapshot mapper. PostgreSQL
// remains at zero, but outstanding overdraft debt in Redis must still veto deletion.
func TestIntegration_BalanceDeleteCascadeRejectsRedisOnlyOverdraftDebt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupTestInfra(t)
	ctx := context.Background()
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	const alias = "@cascade-overdraft"

	params := postgrestestutil.DefaultBalanceParams()
	params.Alias = alias
	params.AssetCode = balanceCacheDeleteAsset
	params.Available = decimal.Zero
	params.OnHold = decimal.Zero
	balanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, accountID, params)

	cacheKey := utils.BalanceInternalKey(infra.orgID, infra.ledgerID, alias+"#default")
	cacheValue := fmt.Sprintf(`{
		"ID":%q,
		"AccountID":%q,
		"Alias":%q,
		"Key":"default",
		"AssetCode":%q,
		"Available":"0",
		"OnHold":"0",
		"Version":0,
		"AccountType":"deposit",
		"AllowSending":1,
		"AllowReceiving":1,
		"OverdraftUsed":"25"
	}`, balanceID.String(), accountID.String(), alias, balanceCacheDeleteAsset)
	require.NoError(t, infra.redisRepo.Set(ctx, cacheKey, cacheValue, 24*60*60))

	// This call exercises the production ListBalanceByKey mapping directly, rather
	// than injecting a domain Balance into the cascade guard.
	cached, err := infra.redisRepo.ListBalanceByKey(ctx, infra.orgID, infra.ledgerID, alias+"#default")
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.True(t, cached.OverdraftUsed.Equal(decimal.NewFromInt(25)))

	err = infra.handler.Command.DeleteAllBalancesByAccountID(ctx,
		infra.orgID, infra.ledgerID, accountID, "integration-cascade-overdraft-delete")
	require.Error(t, err, "Redis-only overdraft debt must reject cascade deletion")
	var conflictErr pkg.EntityConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, cn.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)

	remaining, err := infra.handler.Query.BalanceRepo.Find(ctx, infra.orgID, infra.ledgerID, balanceID)
	require.NoError(t, err)
	assert.NotNil(t, remaining, "rejected cascade must preserve the PostgreSQL balance")
}

// TestIntegration_BalanceDeleteCacheEviction_SingleBalance drives the
// single-balance delete path (DeleteBalance): after a drained balance is deleted
// by id, its cache key is evicted, a delete marker is armed, and a subsequent
// crediting transaction is rejected with 0019 instead of mutating the removed
// balance.
func TestIntegration_BalanceDeleteCacheEviction_SingleBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	wireEmptyLedgerSettings(t, infra)

	ctx := context.Background()

	deletedAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	counterpartyAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	const deletedAlias = "@single-deleted"
	const counterpartyAlias = "@single-funder"

	// The to-be-deleted balance starts at zero available so it is eligible for
	// deletion; the counterparty carries the working funds for the round-trip.
	deletedParams := postgrestestutil.DefaultBalanceParams()
	deletedParams.Alias = deletedAlias
	deletedParams.AssetCode = balanceCacheDeleteAsset
	deletedParams.Available = decimal.Zero
	deletedParams.OnHold = decimal.Zero
	deletedBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, deletedAccountID, deletedParams)

	counterpartyParams := postgrestestutil.DefaultBalanceParams()
	counterpartyParams.Alias = counterpartyAlias
	counterpartyParams.AssetCode = balanceCacheDeleteAsset
	counterpartyParams.Available = decimal.NewFromInt(100)
	counterpartyParams.OnHold = decimal.Zero
	postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, counterpartyAccountID, counterpartyParams)

	// Round-trip to populate both cache entries and leave the to-be-deleted
	// balance back at zero available.
	status, respBody := postTransactionJSON(t, infra, counterpartyAlias, deletedAlias, balanceCacheDeleteAsset, "50")
	require.Equal(t, 201, status, "funding transaction should succeed: %s", respBody)

	status, respBody = postTransactionJSON(t, infra, deletedAlias, counterpartyAlias, balanceCacheDeleteAsset, "50")
	require.Equal(t, 201, status, "return transaction should succeed: %s", respBody)

	cached := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	require.NotNil(t, cached, "balance cache key must exist before delete")
	require.True(t, cached.Available.Equal(decimal.Zero),
		"drained balance available should be zero, got %s", cached.Available.String())

	// Delete the single balance by id.
	err := infra.handler.Command.DeleteBalance(ctx, infra.orgID, infra.ledgerID, deletedBalanceID)
	require.NoError(t, err, "single-balance delete should succeed")

	// (i) The balance cache key is evicted.
	evicted := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	assert.Nil(t, evicted, "balance cache key must be evicted after delete")

	// (ii) The honored-lock delete marker is armed in the dedicated namespace (and its
	// one-release legacy compatibility key).
	deleteMarker, err := infra.redisRepo.Get(ctx, deleteMarkerCacheKey(infra.orgID, infra.ledgerID, deletedAlias, "default"))
	require.NoError(t, err)
	assert.NotEmpty(t, deleteMarker, "delete marker must be present after delete")
	parsedToken, parseErr := uuid.Parse(deleteMarker)
	assert.NoError(t, parseErr, "delete marker must contain a valid UUID ownership token")
	assert.NotEqual(t, uuid.Nil, parsedToken, "delete marker ownership token must be non-nil")

	// (iii) A crediting transaction to the deleted balance is rejected with 0019.
	status, respBody = postTransactionJSON(t, infra, counterpartyAlias, deletedAlias, balanceCacheDeleteAsset, "100")
	assert.Equal(t, 422, status, "transaction on a deleted balance must be rejected, got %d: %s", status, respBody)
	assert.True(t, strings.Contains(respBody, cn.ErrAccountIneligibility.Error()),
		"rejection should carry 0019, got: %s", respBody)

	counterparty := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, counterpartyAlias, "default")
	require.NotNil(t, counterparty, "counterparty balance should still exist")
	assert.True(t, counterparty.Available.Equal(decimal.NewFromInt(100)),
		"counterparty available must be unchanged (100) after the rejected transaction, got %s", counterparty.Available.String())
}

// TestIntegration_BalanceDeleteRejectsRedisFundsWhenPostgresIsZero reproduces
// the write-behind divergence that previously allowed a funded balance to be
// deleted: the HTTP credit updates Redis immediately while the test harness's
// balance-sync worker is not running, so PostgreSQL remains at zero.
func TestIntegration_BalanceDeleteRejectsRedisFundsWhenPostgresIsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	wireEmptyLedgerSettings(t, infra)

	ctx := context.Background()

	deletedAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	counterpartyAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	const deletedAlias = "@redis-funded"
	const counterpartyAlias = "@redis-funder"

	deletedParams := postgrestestutil.DefaultBalanceParams()
	deletedParams.Alias = deletedAlias
	deletedParams.AssetCode = balanceCacheDeleteAsset
	deletedParams.Available = decimal.Zero
	deletedParams.OnHold = decimal.Zero
	deletedBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, deletedAccountID, deletedParams)

	counterpartyParams := postgrestestutil.DefaultBalanceParams()
	counterpartyParams.Alias = counterpartyAlias
	counterpartyParams.AssetCode = balanceCacheDeleteAsset
	counterpartyParams.Available = decimal.NewFromInt(100)
	counterpartyParams.OnHold = decimal.Zero
	postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, counterpartyAccountID, counterpartyParams)

	// The credit changes Redis immediately, but no balance-sync worker is
	// running in this handler-level fixture. This deliberately leaves PG=0 and
	// Redis=50 without polling or sleeping.
	status, respBody := postTransactionJSON(t, infra, counterpartyAlias, deletedAlias, balanceCacheDeleteAsset, "50")
	require.Equal(t, 201, status, "funding transaction should succeed: %s", respBody)

	assert.True(t,
		postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, deletedBalanceID).IsZero(),
		"PostgreSQL available must remain zero before balance sync")
	assert.True(t,
		postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, deletedBalanceID).IsZero(),
		"PostgreSQL on_hold must remain zero before balance sync")
	cached := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	require.NotNil(t, cached, "funded balance must exist in Redis before delete")
	assert.True(t, cached.Available.Equal(decimal.NewFromInt(50)),
		"Redis available must be 50 before delete, got %s", cached.Available.String())

	// The live Redis snapshot must veto deletion even though PostgreSQL still
	// reports zero. The marker acquired for this attempt is released, while the
	// funded cache entry and PostgreSQL row remain untouched.
	err := infra.handler.Command.DeleteBalance(ctx, infra.orgID, infra.ledgerID, deletedBalanceID)
	require.Error(t, err, "funds present only in Redis must reject delete")
	var conflictErr pkg.EntityConflictError
	require.ErrorAs(t, err, &conflictErr)
	assert.Equal(t, cn.ErrBalancesCantBeDeleted.Error(), conflictErr.Code)

	pgBalance, err := infra.handler.Query.BalanceRepo.Find(ctx, infra.orgID, infra.ledgerID, deletedBalanceID)
	require.NoError(t, err, "PostgreSQL balance row must survive rejected delete")
	assert.True(t, pgBalance.Available.IsZero(), "PostgreSQL balance must remain zero")

	cached = getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	require.NotNil(t, cached, "Redis balance must survive rejected delete")
	assert.True(t, cached.Available.Equal(decimal.NewFromInt(50)),
		"Redis available must remain 50 after rejected delete, got %s", cached.Available.String())
	deleteMarker, err := infra.redisRepo.Get(ctx, deleteMarkerCacheKey(infra.orgID, infra.ledgerID, deletedAlias, "default"))
	require.NoError(t, err)
	assert.Empty(t, deleteMarker, "rejected delete must release its own marker")

	// Drain the live funds through the normal transaction path. Redis is now
	// zero while PostgreSQL is still zero, so the next delete is valid.
	status, respBody = postTransactionJSON(t, infra, deletedAlias, counterpartyAlias, balanceCacheDeleteAsset, "50")
	require.Equal(t, 201, status, "draining transaction should succeed: %s", respBody)
	cached = getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	require.NotNil(t, cached, "drained balance should remain cached before successful delete")
	assert.True(t, cached.Available.IsZero(), "Redis available must be zero after draining funds")

	err = infra.handler.Command.DeleteBalance(ctx, infra.orgID, infra.ledgerID, deletedBalanceID)
	require.NoError(t, err, "delete should succeed after Redis funds are drained")

	// A successful delete keeps its marker as the post-commit protection and
	// evicts the now-stale balance snapshot.
	evicted := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, deletedAlias, "default")
	assert.Nil(t, evicted, "balance cache key must be evicted after delete")
	deleteMarker, err = infra.redisRepo.Get(ctx, deleteMarkerCacheKey(infra.orgID, infra.ledgerID, deletedAlias, "default"))
	require.NoError(t, err)
	assert.NotEmpty(t, deleteMarker, "delete marker must be present after delete")
	parsedToken, parseErr := uuid.Parse(deleteMarker)
	assert.NoError(t, parseErr, "delete marker must contain a valid UUID ownership token")
	assert.NotEqual(t, uuid.Nil, parsedToken, "delete marker ownership token must be non-nil")

	// The removed balance remains protected from subsequent transaction
	// mutations, and the rejected transaction must not debit its counterparty.
	status, respBody = postTransactionJSON(t, infra, counterpartyAlias, deletedAlias, balanceCacheDeleteAsset, "100")
	assert.Equal(t, 422, status, "transaction on a deleted balance must be rejected, got %d: %s", status, respBody)
	assert.True(t, strings.Contains(respBody, cn.ErrAccountIneligibility.Error()),
		"rejection should carry 0019, got: %s", respBody)

	counterparty := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, counterpartyAlias, "default")
	require.NotNil(t, counterparty, "counterparty balance should still exist")
	assert.True(t, counterparty.Available.Equal(decimal.NewFromInt(100)),
		"counterparty available must be unchanged (100) after the rejected transaction, got %s", counterparty.Available.String())
}
