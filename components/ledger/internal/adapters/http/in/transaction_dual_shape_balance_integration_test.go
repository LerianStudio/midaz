// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// Balances cached by a newer release carry every field twice, CamelCase and
// lowerCamel. Transactions on them must persist every operation, reading the
// live CamelCase state rather than the lowerCamel copy.
//
// NOT PARALLEL: setupTestInfra and the Huma error hooks share process-global state.

// cacheDualShapeBalance stores the balance row's live state as a dual-shape
// cache entry whose lowerCamel keys carry a different, stale amount.
func cacheDualShapeBalance(t *testing.T, ctx context.Context, infra *testInfra, balanceID uuid.UUID, alias string, available, staleAvailable int64) {
	t.Helper()

	var accountID string

	var version int64

	err := infra.pgContainer.DB.QueryRow(`SELECT account_id, version FROM balance WHERE id = $1`, balanceID).Scan(&accountID, &version)
	require.NoError(t, err)

	entry := fmt.Sprintf(`{"SchemaVersion":2,`+
		`"id":%[1]q,"accountId":%[2]q,"assetCode":"USD","accountType":"deposit","key":"default","alias":%[3]q,`+
		`"available":"%[6]d","onHold":"0","version":"%[5]d","allowSending":true,"allowReceiving":true,"blocked":false,`+
		`"direction":"credit","overdraftUsed":"0","allowOverdraft":false,"overdraftLimitEnabled":false,"overdraftLimit":"0","balanceScope":"transactional",`+
		`"ID":%[1]q,"AccountID":%[2]q,"AssetCode":"USD","AccountType":"deposit","Key":"default","Alias":%[3]q,`+
		`"Available":"%[4]d","OnHold":"0","Version":%[5]d,"AllowSending":1,"AllowReceiving":1,"Blocked":0,`+
		`"Direction":"credit","OverdraftUsed":"0","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`,
		balanceID.String(), accountID, alias, available, version, staleAvailable)

	require.NoError(t, infra.redisRepo.Set(ctx, utils.BalanceInternalKey(infra.orgID, infra.ledgerID, alias+"#default"), entry, 3600))
}

func TestIntegration_TransactionCreate_DualShapeCachedBalancesPersistAllOperations(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()
	sourceID, destinationID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	cacheDualShapeBalance(t, ctx, infra, sourceID, "@src", 1000, 5000)
	cacheDualShapeBalance(t, ctx, infra, destinationID, "@dst", 0, 777)

	app := buildHumaTransactionApp(t, infra.handler, true)

	resp := postTransaction(t, app, v1JSONURL(infra.orgID, infra.ledgerID), equivalentV1Body, "")
	body := readUnusableResultBody(t, resp)
	require.Equal(t, nethttp.StatusCreated, resp.StatusCode, "body: %s", body)

	txID := createdTransactionID(t, body)

	assert.Equal(t, 2, postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID), "both legs must be persisted")

	rows := operationRowsByAlias(fetchOperationRows(t, infra.pgContainer.DB, txID))
	assert.True(t, rows["@src"].AvailableAfter.Equal(decimal.NewFromInt(900)), "source after must derive from the live CamelCase amount, got %s", rows["@src"].AvailableAfter)
	assert.True(t, rows["@dst"].AvailableAfter.Equal(decimal.NewFromInt(100)), "got %s", rows["@dst"].AvailableAfter)

	source := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@src", "default")
	require.NotNil(t, source)
	assert.True(t, source.Available.Equal(decimal.NewFromInt(900)), "got %s", source.Available)
}

func TestIntegration_TransactionCommit_DualShapeCachedBalancesPersistAllLegs(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()
	sourceID, destinationID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)
	cacheDualShapeBalance(t, ctx, infra, sourceID, "@src", 1000, 5000)
	cacheDualShapeBalance(t, ctx, infra, destinationID, "@dst", 0, 777)

	app := buildHumaTransactionApp(t, infra.handler, true)

	created := postTransaction(t, app, v1JSONURL(infra.orgID, infra.ledgerID), unusableResultPendingBody, "")
	createdBody := readUnusableResultBody(t, created)
	require.Equal(t, nethttp.StatusCreated, created.StatusCode, "pending create: %s", createdBody)

	txID := createdTransactionID(t, createdBody)
	require.Equal(t, 1, postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID), "the hold persists its ON_HOLD leg")

	commit := postUnusableResultCommit(t, app, "/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/"+txID.String()+"/commit")
	commitBody := readUnusableResultBody(t, commit)
	require.Equal(t, nethttp.StatusCreated, commit.StatusCode, "commit: %s", commitBody)

	types := map[string]int{}
	for _, row := range fetchOperationRows(t, infra.pgContainer.DB, txID) {
		types[row.Type]++
	}

	assert.Equal(t, 1, types["DEBIT"], "the commit must persist the source debit: %v", types)
	assert.Equal(t, 1, types["CREDIT"], "the commit must persist the destination credit: %v", types)

	destination := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dst", "default")
	require.NotNil(t, destination)
	assert.True(t, destination.Available.Equal(decimal.NewFromInt(100)), "got %s", destination.Available)
}

func TestIntegration_TransactionCreate_UnreadableCachedBalanceIsRefusedBeforeMoving(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()
	sourceID, _ := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	var accountID string
	require.NoError(t, infra.pgContainer.DB.QueryRow(`SELECT account_id FROM balance WHERE id = $1`, sourceID).Scan(&accountID))

	// The Lua script can move this entry, but its numeric ID cannot be decoded.
	sourceKey := utils.BalanceInternalKey(infra.orgID, infra.ledgerID, "@src#default")
	entry := `{"ID":12345,"AccountID":"` + accountID + `","AssetCode":"USD","AccountType":"deposit","Key":"default",` +
		`"Available":"1000","OnHold":"0","Version":1,"AllowSending":1,"AllowReceiving":1,"Direction":"credit",` +
		`"OverdraftUsed":"0","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	require.NoError(t, infra.redisRepo.Set(ctx, sourceKey, entry, 3600))

	app := buildHumaTransactionApp(t, infra.handler, true)

	resp := postTransaction(t, app, v1JSONURL(infra.orgID, infra.ledgerID), equivalentV1Body, "unreadable-cached-balance")
	body := readUnusableResultBody(t, resp)

	assert.Equal(t, nethttp.StatusInternalServerError, resp.StatusCode, "body: %s", body)
	assert.NotContains(t, body, "cannot unmarshal", "the decoder error must not reach the client")

	stored, err := infra.redisRepo.Get(ctx, sourceKey)
	require.NoError(t, err)
	assert.Equal(t, entry, stored, "the cached balance must not move")

	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID))
	assert.Equal(t, 0, countBackupEntriesInLedger(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID), "nothing moved, so no backup entry may remain")

	released, err := infra.redisRepo.SetNX(ctx, utils.IdempotencyInternalKey(infra.orgID, infra.ledgerID, "unreadable-cached-balance"), "", 1)
	require.NoError(t, err)
	assert.True(t, released, "nothing moved, so the idempotency claim must be released")
}

func createdTransactionID(t *testing.T, body string) uuid.UUID {
	t.Helper()

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &created))

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	return id
}

func operationRowsByAlias(rows []operationEconomicRow) map[string]operationEconomicRow {
	byAlias := make(map[string]operationEconomicRow, len(rows))
	for _, row := range rows {
		byAlias[row.AccountAlias] = row
	}

	return byAlias
}
