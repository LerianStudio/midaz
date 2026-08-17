// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	nethttp "net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const remainingLegV1PendingBody = `{
	"description":"remaining leg pending",
	"pending":true,
	"send":{
		"asset":"USD","value":"100",
		"source":{"from":[
			{"accountAlias":"@srcA","amount":{"asset":"USD","value":"60"}},
			{"accountAlias":"@srcB","remaining":"remaining"}
		]},
		"distribute":{"to":[{"accountAlias":"@dstA","amount":{"asset":"USD","value":"100"}}]}
	}
}`

func TestIntegration_TransactionV1Remaining_PendingCommit(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	created := decodeTxResponse(t, postTransaction(t, infra.app, v1JSONURL(infra.orgID, infra.ledgerID), remainingLegV1PendingBody, ""), nethttp.StatusCreated)
	txID := parseTransactionID(t, created)

	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available after pending create")
	requireDecimalEqual(t, decimal.NewFromInt(60), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcA), "@srcA on-hold after pending create")
	requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB available after pending create")
	requireDecimalEqual(t, decimal.NewFromInt(40), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcB), "@srcB on-hold after pending create")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA untouched before commit")

	decodeTxResponse(t, postTransaction(t, infra.app, v1CommitURL(infra.orgID, infra.ledgerID, txID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcA), "@srcA on-hold after commit")
	requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcB), "@srcB on-hold after commit")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA credited after commit")

	ops := fetchOperationRows(t, infra.pgContainer.DB, txID)
	types := make([]string, 0, len(ops))
	for _, op := range ops {
		types = append(types, op.Type)
	}
	assert.ElementsMatch(t, []string{cn.ONHOLD, cn.ONHOLD, cn.DEBIT, cn.DEBIT, cn.CREDIT}, types,
		"commit must settle both resolved remaining/explicit source legs")
}

func TestIntegration_TransactionV1Remaining_PendingCancel(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	created := decodeTxResponse(t, postTransaction(t, infra.app, v1JSONURL(infra.orgID, infra.ledgerID), remainingLegV1PendingBody, ""), nethttp.StatusCreated)
	txID := parseTransactionID(t, created)
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	decodeTxResponse(t, postTransaction(t, infra.app, v1CancelURL(infra.orgID, infra.ledgerID, txID), "", ""), nethttp.StatusCreated)
	assert.Equal(t, cn.CANCELED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA restored after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcA), "@srcA hold released after cancel")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB restored after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcB), "@srcB hold released after cancel")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA untouched after cancel")

	ops := fetchOperationRows(t, infra.pgContainer.DB, txID)
	require.Len(t, ops, 4, "cancel must retain both holds and release both resolved source legs")
	wantAmounts := map[string]decimal.Decimal{
		"@srcA": decimal.NewFromInt(60),
		"@srcB": decimal.NewFromInt(40),
	}
	seen := make(map[string]map[string]bool, len(wantAmounts))
	for _, op := range ops {
		amount, ok := wantAmounts[op.AccountAlias]
		require.Truef(t, ok, "unexpected cancel operation alias %s", op.AccountAlias)
		require.Contains(t, []string{cn.ONHOLD, cn.RELEASE}, op.Type, "unexpected cancel operation type")
		if seen[op.AccountAlias] == nil {
			seen[op.AccountAlias] = make(map[string]bool, 2)
		}
		require.Falsef(t, seen[op.AccountAlias][op.Type], "duplicate %s operation for %s", op.Type, op.AccountAlias)
		seen[op.AccountAlias][op.Type] = true
		requireDecimalEqual(t, amount, op.Amount, "%s %s amount", op.AccountAlias, op.Type)
	}

	for alias := range wantAmounts {
		assert.Truef(t, seen[alias][cn.ONHOLD], "%s hold operation must persist", alias)
		assert.Truef(t, seen[alias][cn.RELEASE], "%s release operation must persist", alias)
	}
}

func TestIntegration_TransactionV1Remaining_DirectRevert(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	ctx := context.Background()
	balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	created := decodeTxResponse(t, postTransaction(t, infra.app, v1JSONURL(infra.orgID, infra.ledgerID), remainingLegV1Body, ""), nethttp.StatusCreated)
	originID := parseTransactionID(t, created)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, originID))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	reversed := decodeTxResponse(t, postTransaction(t, infra.app, v1RevertURL(infra.orgID, infra.ledgerID, originID), "", ""), nethttp.StatusCreated)
	reverseID := parseTransactionID(t, reversed)
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, reverseID))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA restored by revert")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB restored by revert")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA reversed")

	reverseOps := fetchOperationRows(t, infra.pgContainer.DB, reverseID)
	assert.Len(t, reverseOps, 3, "revert must preserve all resolved legs")
	totals := sumOperationAmountsByType(reverseOps)
	requireDecimalEqual(t, decimal.NewFromInt(100), totals[cn.DEBIT], "revert debit total")
	requireDecimalEqual(t, decimal.NewFromInt(100), totals[cn.CREDIT], "revert credit total")
	assert.True(t, totals[cn.DEBIT].Equal(totals[cn.CREDIT]), "revert must remain double-entry balanced")
}

func parseTransactionID(t *testing.T, response map[string]any) uuid.UUID {
	t.Helper()

	id, ok := response["id"].(string)
	require.True(t, ok, "transaction response must contain an id")

	parsed, err := uuid.Parse(id)
	require.NoError(t, err, "transaction response id must be a UUID")

	return parsed
}
