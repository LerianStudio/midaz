// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// overdraftAccount is a seeded account whose default balance may draw overdraft,
// together with the system-managed companion that mirrors its debt.
type overdraftAccount struct {
	balanceID   uuid.UUID
	companionID uuid.UUID
}

// blockUnblockSurface posts a block or unblock through one API version.
type blockUnblockSurface struct {
	name string
	post func(t *testing.T, infra *testInfra, action, sourceAlias, destinationAlias, value string) *nethttp.Response
}

func blockUnblockSurfaces() []blockUnblockSurface {
	return []blockUnblockSurface{
		{
			name: "v1",
			post: func(t *testing.T, infra *testInfra, action, sourceAlias, destinationAlias, value string) *nethttp.Response {
				t.Helper()

				url := v1BlockURL(infra.orgID, infra.ledgerID)
				if action == "unblock" {
					url = v1UnblockURL(infra.orgID, infra.ledgerID)
				}

				body := `{"description":"overdraft rules","send":{"asset":"USD","value":"` + value + `",` +
					`"source":{"from":[{"accountAlias":"` + sourceAlias + `","amount":{"asset":"USD","value":"` + value + `"}}]},` +
					`"distribute":{"to":[{"accountAlias":"` + destinationAlias + `","amount":{"asset":"USD","value":"` + value + `"}}]}}}`

				return postTransaction(t, buildHumaTransactionApp(t, infra.handler, true), url, body, "")
			},
		},
		{
			name: "v2",
			post: func(t *testing.T, infra *testInfra, action, sourceAlias, destinationAlias, value string) *nethttp.Response {
				t.Helper()

				body := `{"description":"overdraft rules","asset":"USD","amount":"` + value + `",` +
					`"debits":[{"alias":"` + sourceAlias + `",` + v2ScopeJSON + `,"amount":"` + value + `"}],` +
					`"credits":[{"alias":"` + destinationAlias + `",` + v2ScopeJSON + `,"amount":"` + value + `"}]}`

				return postV2Create(t, buildHumaV2DirectApp(t, infra.handler), action, infra.orgID, infra.ledgerID, body, "")
			},
		},
	}
}

// TestIntegration_BlockUnblock_OverdraftRules proves, on both API versions and against
// the real engine, that a block never takes on or settles overdraft debt while an unblock
// settles it like any credit and records the repayment as an OVERDRAFT row.
func TestIntegration_BlockUnblock_OverdraftRules(t *testing.T) {
	for _, surface := range blockUnblockSurfaces() {
		t.Run(surface.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see transaction_handler_v2_integration_test.go).
			t.Setenv("ALLOW_INSECURE_TLS", "true")

			infra := setupTestInfra(t)
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			ctx := context.Background()
			db := infra.pgContainer.DB

			t.Run("a block beyond available is refused even when overdraft is allowed", func(t *testing.T) {
				account := seedOverdraftAccount(t, db, infra.orgID, infra.ledgerID, "@od-block-src", 30, 0)
				holding := seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@od-block-hold", 0)

				resp := surface.post(t, infra, "block", "@od-block-src", "@od-block-hold", "50")

				assert.Equal(t, cn.ErrInsufficientFunds.Error(), decodeErrorCode(t, resp, nethttp.StatusUnprocessableEntity))
				drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)
				requireDecimalEqual(t, decimal.NewFromInt(30), postgrestestutil.GetBalanceAvailable(t, db, account.balanceID), "source available")
				requireDecimalEqual(t, decimal.Zero, balanceOverdraftUsed(t, db, account.balanceID), "source overdraft used")
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, db, account.companionID), "source companion")
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, db, holding), "holding available")
			})

			t.Run("a block into an account in overdraft does not repay the debt", func(t *testing.T) {
				source := seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@od-into-src", 100)
				account := seedOverdraftAccount(t, db, infra.orgID, infra.ledgerID, "@od-into-dst", 0, 40)

				created := decodeTxResponse(t, surface.post(t, infra, "block", "@od-into-src", "@od-into-dst", "50"), nethttp.StatusCreated)
				txID := uuid.MustParse(created["id"].(string))
				drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

				requireDecimalEqual(t, decimal.NewFromInt(50), postgrestestutil.GetBalanceAvailable(t, db, source), "source available")
				requireDecimalEqual(t, decimal.NewFromInt(50), postgrestestutil.GetBalanceAvailable(t, db, account.balanceID), "destination available")
				requireDecimalEqual(t, decimal.NewFromInt(40), balanceOverdraftUsed(t, db, account.balanceID), "destination overdraft used")
				requireDecimalEqual(t, decimal.NewFromInt(40), postgrestestutil.GetBalanceAvailable(t, db, account.companionID), "destination companion")
				assert.ElementsMatch(t, []operationTypeRow{
					{Type: cn.BLOCK, Direction: cn.DirectionDebit, Alias: "@od-into-src", Key: "default", Amount: "50"},
					{Type: cn.BLOCK, Direction: cn.DirectionCredit, Alias: "@od-into-dst", Key: "default", Amount: "50"},
				}, operationTypeRows(t, db, txID))
			})

			t.Run("an unblock repays the debt before restoring available", func(t *testing.T) {
				holding := seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@od-unblock-hold", 100)
				account := seedOverdraftAccount(t, db, infra.orgID, infra.ledgerID, "@od-unblock-acc", 0, 10)

				created := decodeTxResponse(t, surface.post(t, infra, "unblock", "@od-unblock-hold", "@od-unblock-acc", "100"), nethttp.StatusCreated)
				txID := uuid.MustParse(created["id"].(string))
				drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, db, holding), "holding available")
				requireDecimalEqual(t, decimal.NewFromInt(90), postgrestestutil.GetBalanceAvailable(t, db, account.balanceID), "account available")
				requireDecimalEqual(t, decimal.Zero, balanceOverdraftUsed(t, db, account.balanceID), "account overdraft used")
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, db, account.companionID), "account companion")
				assert.ElementsMatch(t, []operationTypeRow{
					{Type: cn.UNBLOCK, Direction: cn.DirectionDebit, Alias: "@od-unblock-hold", Key: "default", Amount: "100"},
					{Type: cn.UNBLOCK, Direction: cn.DirectionCredit, Alias: "@od-unblock-acc", Key: "default", Amount: "90"},
					{Type: cn.OVERDRAFT, Direction: cn.DirectionCredit, Alias: "@od-unblock-acc", Key: cn.OverdraftBalanceKey, Amount: "10"},
				}, operationTypeRows(t, db, txID))

				read := getV1Transaction(t, infra, txID)
				assert.Equal(t, []any{"@od-unblock-hold"}, read["source"], "each source alias appears once")
				assert.Equal(t, []any{"@od-unblock-acc"}, read["destination"], "each destination alias appears once")
			})

			t.Run("a smaller unblock repays only part of the debt", func(t *testing.T) {
				seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@od-partial-hold", 5)
				account := seedOverdraftAccount(t, db, infra.orgID, infra.ledgerID, "@od-partial-acc", 0, 10)

				created := decodeTxResponse(t, surface.post(t, infra, "unblock", "@od-partial-hold", "@od-partial-acc", "5"), nethttp.StatusCreated)
				txID := uuid.MustParse(created["id"].(string))
				drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, db, account.balanceID), "account available")
				requireDecimalEqual(t, decimal.NewFromInt(5), balanceOverdraftUsed(t, db, account.balanceID), "account overdraft used")
				requireDecimalEqual(t, decimal.NewFromInt(5), postgrestestutil.GetBalanceAvailable(t, db, account.companionID), "account companion")
				assert.Contains(t, operationTypeRows(t, db, txID),
					operationTypeRow{Type: cn.OVERDRAFT, Direction: cn.DirectionCredit, Alias: "@od-partial-acc", Key: cn.OverdraftBalanceKey, Amount: "5"})
			})
		})
	}
}

// TestIntegration_DirectTransaction_OverdraftUnchangedByBlockRules is the control for
// the block rules: an ordinary direct transaction still draws overdraft and books the
// draw as an OVERDRAFT row.
func TestIntegration_DirectTransaction_OverdraftUnchangedByBlockRules(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	db := infra.pgContainer.DB
	account := seedOverdraftAccount(t, db, infra.orgID, infra.ledgerID, "@od-direct-src", 0, 0)
	seedPlainBalance(t, db, infra.orgID, infra.ledgerID, "@od-direct-dst", 0)

	body := `{"description":"direct draw","send":{"asset":"USD","value":"20",` +
		`"source":{"from":[{"accountAlias":"@od-direct-src","amount":{"asset":"USD","value":"20"}}]},` +
		`"distribute":{"to":[{"accountAlias":"@od-direct-dst","amount":{"asset":"USD","value":"20"}}]}}}`
	created := decodeTxResponse(t, postTransaction(t, buildHumaTransactionApp(t, infra.handler, true), v1JSONURL(infra.orgID, infra.ledgerID), body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(created["id"].(string))
	drainBalanceSync(t, context.Background(), infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, db, account.balanceID), "source available")
	requireDecimalEqual(t, decimal.NewFromInt(20), balanceOverdraftUsed(t, db, account.balanceID), "source overdraft used")
	requireDecimalEqual(t, decimal.NewFromInt(20), postgrestestutil.GetBalanceAvailable(t, db, account.companionID), "source companion")
	assert.Contains(t, operationTypeRows(t, db, txID),
		operationTypeRow{Type: cn.OVERDRAFT, Direction: cn.DirectionDebit, Alias: "@od-direct-src", Key: cn.OverdraftBalanceKey, Amount: "20"})
}

// seedOverdraftAccount seeds a default balance that may draw up to 1000 of overdraft,
// holding the given available amount and outstanding debt, and the companion that
// mirrors that debt.
func seedOverdraftAccount(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, alias string, available, debt int64) overdraftAccount {
	t.Helper()

	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	params := postgrestestutil.DefaultBalanceParams()
	params.Alias, params.AssetCode, params.Available, params.OnHold = alias, "USD", decimal.NewFromInt(available), decimal.Zero
	balanceID := postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, accountID, params)

	_, err := db.Exec(
		`UPDATE balance SET settings = '{"allowOverdraft":true,"overdraftLimitEnabled":true,"overdraftLimit":"1000"}'::jsonb, overdraft_used = $2 WHERE id = $1`,
		balanceID, decimal.NewFromInt(debt),
	)
	require.NoError(t, err, "should enable overdraft on %s", alias)

	companionParams := postgrestestutil.DefaultBalanceParams()
	companionParams.Alias, companionParams.Key, companionParams.AssetCode = alias, cn.OverdraftBalanceKey, "USD"
	companionParams.Available, companionParams.OnHold = decimal.NewFromInt(debt), decimal.Zero
	companionID := postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, accountID, companionParams)

	_, err = db.Exec(`UPDATE balance SET direction = 'debit', settings = '{"balanceScope":"internal"}'::jsonb WHERE id = $1`, companionID)
	require.NoError(t, err, "should shape the overdraft companion of %s", alias)

	return overdraftAccount{balanceID: balanceID, companionID: companionID}
}

func seedPlainBalance(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, alias string, available int64) uuid.UUID {
	t.Helper()

	params := postgrestestutil.DefaultBalanceParams()
	params.Alias, params.AssetCode, params.Available, params.OnHold = alias, "USD", decimal.NewFromInt(available), decimal.Zero

	return postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, uuid.Must(libCommons.GenerateUUIDv7()), params)
}

func balanceOverdraftUsed(t *testing.T, db *sql.DB, balanceID uuid.UUID) decimal.Decimal {
	t.Helper()

	var used decimal.Decimal
	require.NoError(t, db.QueryRow(`SELECT overdraft_used FROM balance WHERE id = $1`, balanceID).Scan(&used))

	return used
}

// operationTypeRow is the part of an operation row the overdraft rules decide.
type operationTypeRow struct {
	Type, Direction, Alias, Key, Amount string
}

func operationTypeRows(t *testing.T, db *sql.DB, txID uuid.UUID) []operationTypeRow {
	t.Helper()

	rows, err := db.Query(`SELECT type, direction, account_alias, balance_key, amount FROM operation WHERE transaction_id = $1`, txID)
	require.NoError(t, err, "should query operation rows")

	defer func() { _ = rows.Close() }()

	var out []operationTypeRow

	for rows.Next() {
		var (
			row    operationTypeRow
			amount decimal.Decimal
		)

		require.NoError(t, rows.Scan(&row.Type, &row.Direction, &row.Alias, &row.Key, &amount))
		row.Amount = amount.String()
		out = append(out, row)
	}

	require.NoError(t, rows.Err())

	return out
}

// decodeErrorCode asserts the status and returns the error code of a refusal.
func decodeErrorCode(t *testing.T, resp *nethttp.Response, wantStatus int) string {
	t.Helper()

	body := drainBody(t, resp)
	require.Equal(t, wantStatus, resp.StatusCode, "unexpected HTTP status; body: %s", string(body))

	var problem struct {
		Code string `json:"code"`
	}

	require.NoError(t, json.Unmarshal(body, &problem), "refusal should be JSON; body: %s", string(body))

	return problem.Code
}

func getV1Transaction(t *testing.T, infra *testInfra, txID uuid.UUID) map[string]any {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodGet,
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/"+txID.String(), nil)

	resp, err := buildHumaTransactionApp(t, infra.handler, true).Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return decodeTxResponse(t, resp, nethttp.StatusOK)
}
