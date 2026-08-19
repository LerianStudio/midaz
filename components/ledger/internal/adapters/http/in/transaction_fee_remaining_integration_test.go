// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"fmt"
	"testing"
	"time"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeeProof_V1RemainingDuplicateAliasPreservesBalanceIdentity(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	accountParams := postgrestestutil.DefaultAccountParams()
	accountParams.Alias = "@payer"
	accountParams.AssetCode = "USD"
	accountParams.Type = "deposit"
	payerAccountID := postgrestestutil.CreateTestAccountWithParams(t, h.db, h.orgID, h.ledgerID, accountParams)

	seedPayerBalance := func(key string) uuid.UUID {
		params := postgrestestutil.DefaultBalanceParams()
		params.Alias = "@payer"
		params.Key = key
		params.AssetCode = "USD"
		params.Available = decimal.NewFromInt(1000)
		params.OnHold = decimal.Zero

		return postgrestestutil.CreateTestBalance(t, h.db, h.orgID, h.ledgerID, payerAccountID, params)
	}

	reservedBalanceID := seedPayerBalance("reserved")
	availableBalanceID := seedPayerBalance("available")
	defaultBalanceID := seedPayerBalance("default")
	receiverBalanceID := h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	feeBalanceID := h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{
		label: "remaining_duplicate_alias",
		fees:  []feeSpec{flatFee("remaining_fee", "@fee_rev", "10", false)},
	})

	body := `{
		"description":"remaining duplicate alias with fee",
		"pending":false,
		"send":{
			"asset":"USD","value":"100",
			"source":{"from":[
				{"accountAlias":"@payer","balanceKey":"reserved","amount":{"asset":"USD","value":"60"}},
				{"accountAlias":"@payer","balanceKey":"available","remaining":"remaining"}
			]},
			"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"100"}}]}
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "remaining+fee create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))
	drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)

	legs := loadLegs(t, h.db, txID)
	require.Len(t, legs, 7, "two authored debits, two fee debits, one transfer credit, and two fee credits must persist")
	requireBalanced(t, legs, "remaining duplicate alias fee transaction")

	assertLegSet := func(alias, key, operationType string, wantAmounts ...decimal.Decimal) {
		t.Helper()

		got := make([]decimal.Decimal, 0, len(wantAmounts))
		for _, leg := range legs {
			if leg.Alias == alias && leg.Key == key && leg.Type == operationType {
				got = append(got, leg.Amount)
			}
		}

		require.Len(t, got, len(wantAmounts), "%s/%s/%s operation count", alias, key, operationType)
		for _, want := range wantAmounts {
			matched := false
			for i, amount := range got {
				if amount.Equal(want) {
					got = append(got[:i], got[i+1:]...)
					matched = true

					break
				}
			}
			require.Truef(t, matched, "%s/%s/%s must contain amount %s", alias, key, operationType, want)
		}
	}

	assertLegSet("@payer", "reserved", cn.DEBIT, decimal.NewFromInt(60))
	assertLegSet("@payer", "available", cn.DEBIT, decimal.NewFromInt(40))
	assertLegSet("@payer", "default", cn.DEBIT, decimal.NewFromInt(6), decimal.NewFromInt(4))
	assertLegSet("@receiver", "default", cn.CREDIT, decimal.NewFromInt(100))
	assertLegSet("@fee_rev", "default", cn.CREDIT, decimal.NewFromInt(6), decimal.NewFromInt(4))

	requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, h.db, reservedBalanceID), "reserved balance after explicit leg")
	requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, h.db, availableBalanceID), "available balance after remaining leg")
	requireDecimalEqual(t, decimal.NewFromInt(990), postgrestestutil.GetBalanceAvailable(t, h.db, defaultBalanceID), "default balance after proportional fee legs")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, h.db, receiverBalanceID), "receiver balance")
	requireDecimalEqual(t, decimal.NewFromInt(10), postgrestestutil.GetBalanceAvailable(t, h.db, feeBalanceID), "fee revenue balance")

	debits := decimal.Zero
	credits := decimal.Zero
	for _, leg := range legs {
		switch leg.Type {
		case cn.DEBIT:
			debits = debits.Add(leg.Amount)
		case cn.CREDIT:
			credits = credits.Add(leg.Amount)
		}
	}
	requireDecimalEqual(t, decimal.NewFromInt(110), debits, "persisted debit total")
	requireDecimalEqual(t, decimal.NewFromInt(110), credits, "persisted credit total")
	assert.True(t, debits.Equal(credits), "persisted operations must remain exact double entry")
}

func TestFeeProof_V1RemainingFeeWithExplicitAccountingRoutes(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	_, err := h.db.Exec(`
		UPDATE ledger
		SET settings = '{"accounting":{"validateRoutes":true}}'::jsonb
		WHERE id = $1 AND organization_id = $2`, h.ledgerID, h.orgID)
	require.NoError(t, err, "enable accounting route validation")

	fixedTime := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	sourceRouteID := uuid.New()
	destinationRouteID := uuid.New()
	transactionRouteID := uuid.New()

	_, err = h.operationRouteRepo.Create(h.ctx(), h.orgID, h.ledgerID, &mmodel.OperationRoute{
		ID:             sourceRouteID,
		OrganizationID: h.orgID,
		LedgerID:       h.ledgerID,
		Title:          "fee source route",
		OperationType:  "source",
		AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{
			Debit: &mmodel.AccountingRubric{Code: "1000", Description: "fee source debit"},
		}},
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	})
	require.NoError(t, err, "create source operation route")

	_, err = h.operationRouteRepo.Create(h.ctx(), h.orgID, h.ledgerID, &mmodel.OperationRoute{
		ID:             destinationRouteID,
		OrganizationID: h.orgID,
		LedgerID:       h.ledgerID,
		Title:          "fee destination route",
		OperationType:  "destination",
		AccountingEntries: &mmodel.AccountingEntries{Direct: &mmodel.AccountingEntry{
			Credit: &mmodel.AccountingRubric{Code: "2000", Description: "fee destination credit"},
		}},
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	})
	require.NoError(t, err, "create destination operation route")

	_, err = h.transactionRouteRepo.Create(h.ctx(), h.orgID, h.ledgerID, &mmodel.TransactionRoute{
		ID:             transactionRouteID,
		OrganizationID: h.orgID,
		LedgerID:       h.ledgerID,
		Title:          "remaining fee route",
		OperationRoutes: []mmodel.OperationRoute{
			{ID: sourceRouteID},
			{ID: destinationRouteID},
		},
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	})
	require.NoError(t, err, "create transaction route")

	accountParams := postgrestestutil.DefaultAccountParams()
	accountParams.Alias = "@payer"
	accountParams.AssetCode = "USD"
	accountParams.Type = "deposit"
	payerAccountID := postgrestestutil.CreateTestAccountWithParams(t, h.db, h.orgID, h.ledgerID, accountParams)
	seedPayerBalance := func(key string) uuid.UUID {
		params := postgrestestutil.DefaultBalanceParams()
		params.Alias = "@payer"
		params.Key = key
		params.AssetCode = "USD"
		params.Available = decimal.NewFromInt(1000)
		params.OnHold = decimal.Zero

		return postgrestestutil.CreateTestBalance(t, h.db, h.orgID, h.ledgerID, payerAccountID, params)
	}
	reservedBalanceID := seedPayerBalance("reserved")
	availableBalanceID := seedPayerBalance("available")
	defaultBalanceID := seedPayerBalance("default")
	receiverBalanceID := h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	feeBalanceID := h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	legacyFrom := uuid.NewString()
	legacyTo := uuid.NewString()
	sourceRouteString := sourceRouteID.String()
	destinationRouteString := destinationRouteID.String()
	fee := flatFee("accounting_remaining_fee", "@fee_rev", "10", false)
	fee.routeFrom = &legacyFrom
	fee.routeTo = &legacyTo
	fee.operationRouteFromID = &sourceRouteString
	fee.operationRouteToID = &destinationRouteString
	h.seedPackage(t, packageSpec{label: "accounting_remaining", fees: []feeSpec{fee}})

	body := fmt.Sprintf(`{
		"description":"remaining fee with accounting routes",
		"pending":false,
		"routeId":"%s",
		"send":{
			"asset":"USD","value":"100",
			"source":{"from":[
				{"accountAlias":"@payer","balanceKey":"reserved","routeId":"%s","amount":{"asset":"USD","value":"60"}},
				{"accountAlias":"@payer","balanceKey":"available","routeId":"%s","remaining":"remaining"}
			]},
			"distribute":{"to":[{"accountAlias":"@receiver","routeId":"%s","amount":{"asset":"USD","value":"100"}}]}
		}
	}`, transactionRouteID, sourceRouteID, sourceRouteID, destinationRouteID)

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "remaining+fee accounting create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))
	drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)

	legs := loadLegs(t, h.db, txID)
	require.Len(t, legs, 7)
	requireBalanced(t, legs, "remaining fee with accounting routes")

	for _, leg := range legs {
		require.NotNil(t, leg.RouteID, "%s/%s operation route", leg.Alias, leg.Type)

		switch leg.Type {
		case cn.DEBIT:
			assert.Equal(t, sourceRouteID, *leg.RouteID)
		case cn.CREDIT:
			assert.Equal(t, destinationRouteID, *leg.RouteID)
		}

		if leg.Alias == "@payer" && leg.Key == "default" {
			require.NotNil(t, leg.Route)
			assert.Equal(t, legacyFrom, *leg.Route, "UUID-looking debit label remains legacy Route")
		}

		if leg.Alias == "@fee_rev" {
			require.NotNil(t, leg.Route)
			assert.Equal(t, legacyTo, *leg.Route, "UUID-looking credit label remains legacy Route")
		}
	}

	requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, h.db, reservedBalanceID), "reserved balance")
	requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, h.db, availableBalanceID), "available balance")
	requireDecimalEqual(t, decimal.NewFromInt(990), postgrestestutil.GetBalanceAvailable(t, h.db, defaultBalanceID), "default fee balance")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, h.db, receiverBalanceID), "receiver balance")
	requireDecimalEqual(t, decimal.NewFromInt(10), postgrestestutil.GetBalanceAvailable(t, h.db, feeBalanceID), "fee revenue balance")
}

func TestFeeProof_ZeroFeeIsNoOp(t *testing.T) {
	tests := []struct {
		name string
		fee  feeSpec
	}{
		{name: "flat zero", fee: flatFee("flat_zero", "@fee_rev", "0", false)},
		{name: "percentage zero", fee: percentualFee("percentage_zero", "@fee_rev", "0", false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := setupFeeHarness(t)
			app := h.newApp()
			payerBalanceID := h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(1000), "deposit")
			receiverBalanceID := h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
			feeBalanceID := h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
			h.seedPackage(t, packageSpec{label: "zero_fee", fees: []feeSpec{tt.fee}})

			body := `{
				"description":"zero fee no-op",
				"pending":false,
				"send":{
					"asset":"USD","value":"100",
					"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"USD","value":"100"}}]},
					"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"100"}}]}
				}
			}`

			resp := h.createJSON(t, app, body, nil)
			require.Equalf(t, 201, resp.status, "zero fee create must succeed: %s", string(resp.rawBody))

			txID := mustTxID(t, resp)
			drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)
			legs := loadLegs(t, h.db, txID)
			require.Len(t, legs, 2, "a zero result must persist only the authored transfer legs")
			requireBalanced(t, legs, "zero fee transaction")
			requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, h.db, payerBalanceID), "payer balance")
			requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, h.db, receiverBalanceID), "receiver balance")
			requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, h.db, feeBalanceID), "fee revenue balance")
		})
	}
}
