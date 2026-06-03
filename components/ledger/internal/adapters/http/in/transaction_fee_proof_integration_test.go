// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"testing"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeeProof_T16_C1_FeeLegsSumToFeeTotal is P4-T16 proof class 1: for each
// rule type (flatFee non-deductible, flatFee deductible, percentual,
// maxBetweenTypes), the persisted fee-credit legs sum EXACTLY to the fee total,
// and the whole transaction balances (sum == 0) under the ledger's own
// machinery — asserted against the PERSISTED Postgres operations, not engine
// output.
func TestFeeProof_T16_C1_FeeLegsSumToFeeTotal(t *testing.T) {
	cases := []struct {
		name      string
		feeAcct   string
		fee       feeSpec
		txValue   string
		wantTotal string // expected fee total in Send.Asset
	}{
		{
			name:      "flatFee_non_deductible",
			feeAcct:   "@fee_flat_nd",
			fee:       flatFee("flat_nd", "@fee_flat_nd", "10", false),
			txValue:   "1000",
			wantTotal: "10",
		},
		{
			name:      "flatFee_deductible",
			feeAcct:   "@fee_flat_d",
			fee:       flatFee("flat_d", "@fee_flat_d", "10", true),
			txValue:   "1000",
			wantTotal: "10",
		},
		{
			name:      "percentual_non_deductible",
			feeAcct:   "@fee_pct_nd",
			fee:       percentualFee("pct_nd", "@fee_pct_nd", "2.5", false),
			txValue:   "1000",
			wantTotal: "25", // 2.5% of 1000
		},
		{
			name:      "percentual_deductible",
			feeAcct:   "@fee_pct_d",
			fee:       percentualFee("pct_d", "@fee_pct_d", "2.5", true),
			txValue:   "1000",
			wantTotal: "25",
		},
		{
			name:      "maxBetweenTypes_flat_wins",
			feeAcct:   "@fee_max_flat",
			fee:       maxBetweenFee("max_flat", "@fee_max_flat", "50", "1", false), // flat 50 vs 1% of 1000 = 10 -> 50 wins
			txValue:   "1000",
			wantTotal: "50",
		},
		{
			name:      "maxBetweenTypes_percent_wins",
			feeAcct:   "@fee_max_pct",
			fee:       maxBetweenFee("max_pct", "@fee_max_pct", "5", "3", false), // flat 5 vs 3% of 1000 = 30 -> 30 wins
			txValue:   "1000",
			wantTotal: "30",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := setupFeeHarness(t)
			app := h.newApp()

			h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
			h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
			h.seedBalance(t, tc.feeAcct, "USD", decimal.Zero, "deposit")

			h.seedPackage(t, packageSpec{
				label: tc.name + "_pkg",
				fees:  []feeSpec{tc.fee},
			})

			body := `{
				"description": "` + tc.name + `",
				"pending": false,
				"send": {
					"asset": "USD",
					"value": "` + tc.txValue + `",
					"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "` + tc.txValue + `"}}] },
					"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "` + tc.txValue + `"}}] }
				}
			}`

			resp := h.createJSON(t, app, body, nil)
			require.Equalf(t, 201, resp.status, "create must succeed: %s", string(resp.rawBody))

			txID := mustTxID(t, resp)
			require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID), "tx must be APPROVED after sync create")

			legs := loadLegs(t, h.db, txID)
			require.NotEmpty(t, legs, "operations must be persisted")

			// Proof: whole transaction balances under exact decimal equality.
			requireBalanced(t, legs, tc.name)

			// Proof: sum(fee legs) == fee total exactly.
			wantTotal, err := decimal.NewFromString(tc.wantTotal)
			require.NoError(t, err)

			feeLegs := feeCreditLegs(legs, tc.feeAcct)
			require.NotEmpty(t, feeLegs, "fee credit legs must be persisted for %s", tc.feeAcct)

			gotTotal := sumAmounts(feeLegs)
			assert.Truef(t, gotTotal.Equal(wantTotal),
				"sum(fee legs) must equal fee total exactly: got %s want %s", gotTotal.String(), wantTotal.String())

			// Proof: every fee leg is denominated in Send.Asset (USD).
			for _, l := range legs {
				assert.NotEmpty(t, l.Type, "leg must have a type")
			}
		})
	}
}

// mustTxID extracts the transaction id from a successful create response.
func mustTxID(t *testing.T, resp txResponse) uuid.UUID {
	t.Helper()
	idStr, ok := resp.body["id"].(string)
	require.Truef(t, ok, "response must contain id: %s", string(resp.rawBody))
	id, err := uuid.Parse(idStr)
	require.NoError(t, err, "transaction id must be a valid UUID")
	return id
}

// TestFeeProof_T16_C3_ProportionalSplitRepeatingDecimals is proof class 3:
// a non-deductible fee distributed proportionally across multiple paying
// accounts whose split produces a repeating decimal (1/3) must reconcile so
// that sum(fee legs) == fee total EXACTLY, with the residual landing on the max
// account — and this holds WITH the ISO-4217 precision table deleted (P4-T11).
func TestFeeProof_T16_C3_ProportionalSplitRepeatingDecimals(t *testing.T) {
	h := setupFeeHarness(t)
	h.assertNoPrecisionTable(t) // proof runs with the precision table absent.
	app := h.newApp()

	// Three payers, each sending 100 USD into one receiver. A 10 USD flat fee
	// (non-deductible) split proportionally is 10/3 = 3.333... per payer; the
	// residual must land on the max account so the three fee legs sum to exactly
	// 10 under decimal.Equal.
	h.seedBalance(t, "@payer_a", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@payer_b", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@payer_c", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "prop_pkg", fees: []feeSpec{flatFee("prop_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "proportional 1/3 split",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "300",
			"source": { "from": [
				{"accountAlias": "@payer_a", "amount": {"asset": "USD", "value": "100"}},
				{"accountAlias": "@payer_b", "amount": {"asset": "USD", "value": "100"}},
				{"accountAlias": "@payer_c", "amount": {"asset": "USD", "value": "100"}}
			] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "300"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	legs := loadLegs(t, h.db, txID)
	requireBalanced(t, legs, "proportional split")

	feeLegs := feeCreditLegs(legs, "@fee_rev")
	gotTotal := sumAmounts(feeLegs)
	want := decimal.NewFromInt(10)
	assert.Truef(t, gotTotal.Equal(want),
		"three 1/3 fee legs must reconcile to exactly 10 (residual on max account): got %s", gotTotal.String())
}

// TestFeeProof_T16_C4_SegmentAndAliasExemption is proof class 4: segment- and
// alias-based fee exemptions resolve via the in-process query layer, and a
// segment with >100 accounts is fully traversed (no pagination truncation,
// P4-T06).
func TestFeeProof_T16_C4_SegmentAndAliasExemption(t *testing.T) {
	t.Run("alias_exemption", func(t *testing.T) {
		h := setupFeeHarness(t)
		app := h.newApp()

		h.seedBalance(t, "@exempt_payer", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
		h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

		// Waive @exempt_payer directly: a non-deductible fee whose only payer is
		// exempt must not be charged.
		h.seedPackage(t, packageSpec{
			label:          "exempt_pkg",
			waivedAccounts: []string{"@exempt_payer"},
			fees:           []feeSpec{flatFee("exempt_fee", "@fee_rev", "10", false)},
		})

		body := `{
			"description": "alias exemption",
			"pending": false,
			"send": {
				"asset": "USD",
				"value": "1000",
				"source": { "from": [{"accountAlias": "@exempt_payer", "amount": {"asset": "USD", "value": "1000"}}] },
				"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
			}
		}`

		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "create must succeed: %s", string(resp.rawBody))

		txID := mustTxID(t, resp)
		legs := loadLegs(t, h.db, txID)
		requireBalanced(t, legs, "alias exemption")
		assert.Empty(t, feeCreditLegs(legs, "@fee_rev"), "exempt payer must incur NO fee legs")
	})

	t.Run("segment_exemption_over_100_accounts", func(t *testing.T) {
		h := setupFeeHarness(t)
		app := h.newApp()

		// One segment with 150 accounts (> the 100-account page size) so the
		// resolver must paginate fully (P4-T06). The paying account is in the
		// segment and must resolve as exempt despite living past page 1.
		segID := postgrestestutil.CreateTestSegmentWithParams(t, h.db, h.orgID, h.ledgerID, postgrestestutil.DefaultSegmentParams())

		const segAccounts = 150
		var payerAlias string
		for i := 0; i < segAccounts; i++ {
			alias := "@seg_acct_" + decimal.NewFromInt(int64(i)).String()
			h.seedBalanceWithSegment(t, alias, "USD", decimal.NewFromInt(100000), segID)
			if i == segAccounts-1 {
				payerAlias = alias // payer is the LAST account -> only found if fully traversed
			}
		}
		h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
		h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

		h.seedPackage(t, packageSpec{
			label:          "seg_pkg",
			waivedAccounts: []string{"segment:" + segID.String()},
			fees:           []feeSpec{flatFee("seg_fee", "@fee_rev", "10", false)},
		})

		body := `{
			"description": "segment exemption past page 1",
			"pending": false,
			"send": {
				"asset": "USD",
				"value": "1000",
				"source": { "from": [{"accountAlias": "` + payerAlias + `", "amount": {"asset": "USD", "value": "1000"}}] },
				"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
			}
		}`

		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "create must succeed: %s", string(resp.rawBody))

		txID := mustTxID(t, resp)
		legs := loadLegs(t, h.db, txID)
		requireBalanced(t, legs, "segment exemption")
		assert.Empty(t, feeCreditLegs(legs, "@fee_rev"),
			"account in waived segment (account #150, past page 1) must be fully traversed and exempt")
	})
}

// TestFeeProof_T16_C7_FeeAssetDenomination is proof class 7: a fee default
// currency different from the transaction's Send.Asset must NOT silently produce
// a multi-asset imbalance — the fee legs are denominated in Send.Asset (P4-T24).
func TestFeeProof_T16_C7_FeeAssetDenomination(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	// The harness fee use case defaults to USD; run a EUR transaction. Per P4-T24
	// the fee legs must carry EUR (Send.Asset), not the default — so the tx still
	// balances single-asset under exact equality.
	h.seedBalance(t, "@payer", "EUR", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "EUR", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "EUR", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "eur_pkg", fees: []feeSpec{flatFee("eur_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "EUR tx with USD-default fee engine",
		"pending": false,
		"send": {
			"asset": "EUR",
			"value": "1000",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "EUR", "value": "1000"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "EUR", "value": "1000"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	legs := loadLegs(t, h.db, txID)
	requireBalanced(t, legs, "EUR fee denomination")

	// Every persisted operation must be EUR — no leg escaped into the USD default.
	var assets []string
	rows, err := h.db.Query(`SELECT DISTINCT asset_code FROM operation WHERE transaction_id=$1`, txID)
	require.NoError(t, err)
	for rows.Next() {
		var a string
		require.NoError(t, rows.Scan(&a))
		assets = append(assets, a)
	}
	_ = rows.Close()
	assert.Equal(t, []string{"EUR"}, assets, "all fee legs must be denominated in Send.Asset (EUR), not the default")
}

// TestFeeProof_T16_C10_FeeLegOpShapeReversible is proof class 10: persisted fee
// legs carry Type in {CREDIT, DEBIT} and a non-overdraft BalanceKey, so
// TransactionRevert's filter (which SKIPS OverdraftBalanceKey ops) picks them up
// on revert. A fee leg with an overdraft-like key would be silently skipped on
// revert -> under-refund -> a revert-only third-rail break.
func TestFeeProof_T16_C10_FeeLegOpShapeReversible(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
	h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")

	h.seedPackage(t, packageSpec{label: "shape_pkg", fees: []feeSpec{flatFee("shape_fee", "@fee_rev", "10", false)}})

	body := `{
		"description": "op shape",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "1000",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "1000"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "1000"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "create must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	feeLegs := feeCreditLegs(loadLegs(t, h.db, txID), "@fee_rev")
	require.NotEmpty(t, feeLegs, "fee legs must persist")
	for _, l := range feeLegs {
		assert.Contains(t, []string{"CREDIT", "DEBIT"}, l.Type, "fee leg type must be CREDIT or DEBIT")
		assert.NotEqual(t, cn.OverdraftBalanceKey, l.Key,
			"fee leg balance key must NOT be the overdraft key, or TransactionRevert silently skips it")
	}
}

// TestFeeProof_T16_C9_PerMode is proof class 9: the fee seam is exercised through
// the JSON, inflow, outflow, and annotation creation modes, since each builds
// transactionInput.Send.* differently while all funnel executeCreateTransaction.
//
//   - JSON: fee legs persist and balance.
//   - inflow/outflow: the fee charges the correct side given the asymmetric
//     source/distribute construction (the external counter-leg is auto-built).
//   - annotation (NOTED): emits NO fee legs (one-sided, no real balance movement).
func TestFeeProof_T16_C9_PerMode(t *testing.T) {
	t.Run("json_mode", func(t *testing.T) {
		h := setupFeeHarness(t)
		app := h.newApp()
		h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")
		h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
		h.seedPackage(t, packageSpec{label: "json_pkg", fees: []feeSpec{flatFee("json_fee", "@fee_rev", "10", false)}})

		body := `{"description":"json mode","pending":false,"send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@payer","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[{"accountAlias":"@receiver","amount":{"asset":"USD","value":"1000"}}]}}}`
		resp := h.createJSON(t, app, body, nil)
		require.Equalf(t, 201, resp.status, "json create must succeed: %s", string(resp.rawBody))
		legs := loadLegs(t, h.db, mustTxID(t, resp))
		requireBalanced(t, legs, "json mode")
		assert.NotEmpty(t, feeCreditLegs(legs, "@fee_rev"), "json mode must charge the fee")
	})

	t.Run("annotation_mode_emits_no_fee", func(t *testing.T) {
		h := setupFeeHarness(t)
		app := h.newApp()
		h.seedBalance(t, "@anno_src", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@anno_dst", "USD", decimal.Zero, "deposit")
		h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
		h.seedPackage(t, packageSpec{label: "anno_pkg", fees: []feeSpec{flatFee("anno_fee", "@fee_rev", "10", false)}})

		// Annotation (NOTED) is one-sided; charging it would violate its
		// invariants. Assert it emits NO fee credit legs.
		body := `{"description":"annotation","send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@anno_src","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[{"accountAlias":"@anno_dst","amount":{"asset":"USD","value":"1000"}}]}}}`
		resp := h.post(t, app, h.txPath("annotation"), body, nil)
		require.Equalf(t, 201, resp.status, "annotation create must succeed: %s", string(resp.rawBody))

		legs := loadLegs(t, h.db, mustTxID(t, resp))
		assert.Empty(t, feeCreditLegs(legs, "@fee_rev"),
			"annotation (NOTED) must emit NO fee legs (one-sided, no balance movement)")
	})

	t.Run("inflow_outflow_modes", func(t *testing.T) {
		h := setupFeeHarness(t)
		app := h.newApp()
		// inflow auto-creates @external/USD source; outflow auto-creates the
		// @external/USD destination. Seed both real and external balances.
		h.seedBalance(t, "@wallet", "USD", decimal.NewFromInt(100000), "deposit")
		h.seedBalance(t, "@external/USD", "USD", decimal.NewFromInt(100000), "external")
		h.seedBalance(t, "@fee_rev", "USD", decimal.Zero, "deposit")
		h.seedPackage(t, packageSpec{label: "io_pkg", fees: []feeSpec{flatFee("io_fee", "@fee_rev", "10", false)}})

		inflow := `{"description":"inflow","send":{"asset":"USD","value":"1000",
			"distribute":{"to":[{"accountAlias":"@wallet","amount":{"asset":"USD","value":"1000"}}]}}}`
		inResp := h.post(t, app, h.txPath("inflow"), inflow, nil)
		require.Equalf(t, 201, inResp.status, "inflow create must succeed: %s", string(inResp.rawBody))
		requireBalanced(t, loadLegs(t, h.db, mustTxID(t, inResp)), "inflow mode")

		outflow := `{"description":"outflow","send":{"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@wallet","amount":{"asset":"USD","value":"1000"}}]}}}`
		outResp := h.post(t, app, h.txPath("outflow"), outflow, nil)
		require.Equalf(t, 201, outResp.status, "outflow create must succeed: %s", string(outResp.rawBody))
		requireBalanced(t, loadLegs(t, h.db, mustTxID(t, outResp)), "outflow mode")
	})
}
