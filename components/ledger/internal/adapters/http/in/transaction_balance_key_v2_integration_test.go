// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	nethttp "net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func seedAdditionalBalanceForV2(
	t *testing.T,
	db *sql.DB,
	orgID, ledgerID uuid.UUID,
	alias, key string,
	available int64,
) uuid.UUID {
	t.Helper()

	var accountID uuid.UUID
	err := db.QueryRow(`
		SELECT account_id
		FROM balance
		WHERE organization_id = $1 AND ledger_id = $2 AND alias = $3 AND key = $4 AND deleted_at IS NULL
	`, orgID, ledgerID, alias, cn.DefaultBalanceKey).Scan(&accountID)
	require.NoError(t, err, "find account for additional balance")

	params := postgrestestutil.DefaultBalanceParams()
	params.Alias = alias
	params.Key = key
	params.AssetCode = "USD"
	params.Available = decimal.NewFromInt(available)
	params.OnHold = decimal.Zero

	return postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, accountID, params)
}

func markBalanceInternalForV2(t *testing.T, db *sql.DB, balanceID uuid.UUID) {
	t.Helper()

	result, err := db.Exec(`UPDATE balance SET settings = '{"balanceScope":"internal"}'::jsonb WHERE id = $1`, balanceID)
	require.NoError(t, err, "mark balance as internal")
	affected, err := result.RowsAffected()
	require.NoError(t, err, "read internal-balance update count")
	require.EqualValues(t, 1, affected)
}

func v2BalanceKeyBody(description, debitAlias, debitKey, creditAlias, creditKey string) string {
	leg := func(alias, key string) string {
		balanceKey := ""
		if key != "" {
			balanceKey = `,"balanceKey":"` + key + `"`
		}

		return `{"alias":"` + alias + `",` + v2ScopeJSON + `,"amount":"100"` + balanceKey + `}`
	}

	return `{"description":"` + description + `","asset":"USD","amount":"100",` +
		`"debits":[` + leg(debitAlias, debitKey) + `],"credits":[` + leg(creditAlias, creditKey) + `]}`
}

func requireResponseBalanceKey(t *testing.T, response map[string]any, alias, operationType, want string) {
	t.Helper()

	operations := responseOperationsByAliasAndType(t, response)
	raw, ok := operations[alias+"/"+operationType]
	require.Truef(t, ok, "response must contain %s/%s", alias, operationType)
	operation, ok := raw.(map[string]any)
	require.True(t, ok, "response operation must be an object")
	assert.Equal(t, want, operation["balanceKey"])
}

func requireCachedBalanceAvailable(
	t *testing.T,
	ctx context.Context,
	infra *testInfra,
	ledgerID uuid.UUID,
	alias, key string,
	want int64,
) {
	t.Helper()

	balance := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, ledgerID, alias, key)
	require.NotNilf(t, balance, "live balance %s#%s must exist", alias, key)
	requireDecimalEqual(t, decimal.NewFromInt(want), balance.Available, alias+"#"+key+" available")
}

func TestIntegration_TransactionV2BalanceKey(t *testing.T) {
	// NOT parallel: the v2 app builder installs process-global Huma hooks.
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	app := buildHumaV2DirectApp(t, infra.handler)
	ctx := context.Background()

	t.Run("direct response and revert preserve named balance", func(t *testing.T) {
		sourceDefaultID, destinationDefaultID := seedTransfer(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@named-source", "@named-destination", 1000,
		)
		sourceFoodID := seedAdditionalBalanceForV2(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@named-source", "food", 500,
		)
		created := decodeTxResponse(t, postV2Create(
			t, app, "direct", infra.orgID, infra.ledgerID,
			v2BalanceKeyBody("named direct", "@named-source", "food", "@named-destination", ""), "",
		), nethttp.StatusCreated)
		requireResponseBalanceKey(t, created, "@named-source", cn.DEBIT, "food")
		requireResponseBalanceKey(t, created, "@named-destination", cn.CREDIT, cn.DefaultBalanceKey)

		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceDefaultID))
		requireDecimalEqual(t, decimal.NewFromInt(400), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceFoodID))
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destinationDefaultID))

		originID := uuid.MustParse(created["id"].(string))
		reverted := decodeTxResponse(
			t,
			postTransaction(t, app, v2RevertURL(infra.orgID, infra.ledgerID, originID), "", ""),
			nethttp.StatusCreated,
		)
		requireResponseBalanceKey(t, reverted, "@named-source", cn.CREDIT, "food")
		requireResponseBalanceKey(t, reverted, "@named-destination", cn.DEBIT, cn.DefaultBalanceKey)

		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceDefaultID))
		requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceFoodID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destinationDefaultID))
	})

	t.Run("hold and commit reserve only the named balance", func(t *testing.T) {
		sourceDefaultID, destinationDefaultID := seedTransfer(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@hold-source", "@hold-destination", 1000,
		)
		sourceFoodID := seedAdditionalBalanceForV2(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@hold-source", "food", 500,
		)

		held := decodeTxResponse(t, postV2Create(
			t, app, "hold", infra.orgID, infra.ledgerID,
			v2BalanceKeyBody("named hold", "@hold-source", "food", "@hold-destination", ""), "",
		), nethttp.StatusCreated)
		requireResponseBalanceKey(t, held, "@hold-source", cn.ONHOLD, "food")
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceDefaultID))
		requireDecimalEqual(t, decimal.NewFromInt(400), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceFoodID))
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, sourceFoodID))

		heldID := uuid.MustParse(held["id"].(string))
		committed := decodeTxResponse(
			t,
			postTransaction(t, app, v2CommitURL(infra.orgID, infra.ledgerID, heldID), "", ""),
			nethttp.StatusCreated,
		)
		requireResponseBalanceKey(t, committed, "@hold-source", cn.DEBIT, "food")
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceDefaultID))
		requireDecimalEqual(t, decimal.NewFromInt(400), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceFoodID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, sourceFoodID))
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destinationDefaultID))
	})

	t.Run("same account can transfer between distinct balances", func(t *testing.T) {
		walletDefaultID, _ := seedTransfer(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@wallet", "@wallet-unused", 1000,
		)
		walletFoodID := seedAdditionalBalanceForV2(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@wallet", "food", 500,
		)

		response := decodeTxResponse(t, postV2Create(
			t, app, "direct", infra.orgID, infra.ledgerID,
			v2BalanceKeyBody("between balances", "@wallet", "food", "@wallet", cn.DefaultBalanceKey), "",
		), nethttp.StatusCreated)
		requireResponseBalanceKey(t, response, "@wallet", cn.DEBIT, "food")
		requireResponseBalanceKey(t, response, "@wallet", cn.CREDIT, cn.DefaultBalanceKey)
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		requireDecimalEqual(t, decimal.NewFromInt(400), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, walletFoodID))
		requireDecimalEqual(t, decimal.NewFromInt(1100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, walletDefaultID))
	})

	t.Run("caller cannot target the internal overdraft balance", func(t *testing.T) {
		seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@internal-source", "@internal-destination", 1000)
		internalID := seedAdditionalBalanceForV2(
			t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@internal-source", cn.OverdraftBalanceKey, 100,
		)
		markBalanceInternalForV2(t, infra.pgContainer.DB, internalID)

		response := postV2Create(
			t, app, "direct", infra.orgID, infra.ledgerID,
			v2BalanceKeyBody("internal rejected", "@internal-source", cn.OverdraftBalanceKey, "@internal-destination", ""), "",
		)
		body := drainBody(t, response)
		require.Equal(t, nethttp.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, cn.ErrDirectOperationOnInternalBalance.Error())
	})

	t.Run("unknown balance key keeps the established engine error", func(t *testing.T) {
		seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@missing-source", "@missing-destination", 1000)

		response := postV2Create(
			t, app, "direct", infra.orgID, infra.ledgerID,
			v2BalanceKeyBody("missing balance", "@missing-source", "missing", "@missing-destination", ""), "",
		)
		body := drainBody(t, response)
		require.Equal(t, nethttp.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, cn.ErrAccountIneligibility.Error())
	})
}
