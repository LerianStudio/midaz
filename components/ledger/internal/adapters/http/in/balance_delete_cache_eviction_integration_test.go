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
	"net/http/httptest"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/ledger"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
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
// With the fix, a delete plants a short-lived "<balanceKey>:deleted" delete marker,
// soft-deletes the PostgreSQL row, then evicts (Del) the balance cache key. A
// later transaction can no longer find the balance (cache evicted + row
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

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/json",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
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
// delete is armed: the balance's internal cache key plus the ":deleted" suffix.
func deleteMarkerCacheKey(orgID, ledgerID uuid.UUID, alias, key string) string {
	return utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+key) + ":deleted"
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

	// (ii) The honored-lock delete marker is armed on the separate ":deleted" key.
	deleteMarker, err := infra.redisRepo.Get(ctx, deleteMarkerCacheKey(infra.orgID, infra.ledgerID, deletedAlias, "default"))
	require.NoError(t, err)
	assert.Equal(t, "1", deleteMarker, "delete marker must be present after delete")

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

	// (ii) The honored-lock delete marker is armed.
	deleteMarker, err := infra.redisRepo.Get(ctx, deleteMarkerCacheKey(infra.orgID, infra.ledgerID, deletedAlias, "default"))
	require.NoError(t, err)
	assert.Equal(t, "1", deleteMarker, "delete marker must be present after delete")

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
