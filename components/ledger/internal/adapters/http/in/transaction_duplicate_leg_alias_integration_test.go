// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	nethttp "net/http"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// A transaction body may name the SAME account twice on the SAME side, each leg carrying its own
// value. The create funnel resolves per-leg values into a map keyed by the leg's account alias
// while totalling every leg unconditionally, so two legs sharing one bare alias would collapse
// into a single map entry — one leg's value overwriting the other's — while the balance check
// still saw the sum of both. What keeps the two legs apart is the composite per-leg alias
// ("<index>#alias#balanceKey"), stamped onto every leg before the authoritative resolution runs:
// each leg resolves independently of its siblings, whatever alias they name.
//
// The guarantee is asserted, not the failure mode of losing it. Removing the composite alias has
// been observed to yield a 422/0019 account-ineligibility rejection on both surfaces — one
// spelling of "the guard is gone", not proof that no way of losing it could commit silently. The
// per-leg assertions stand on their own terms regardless: they also catch a value corruption such
// as both legs resolving to 500.
//
// This file locks the observable consequence, on both surfaces that can spell the shape: the
// repeated account is debited the SUM of its legs, each leg persists its own operation row
// carrying that leg's own value, and the persisted debits equal the persisted credits.
//
// Operation ORDER is deliberately never asserted. Two legs sharing an account and balance key
// have no defined relative order in the persisted set, so every assertion here is
// order-independent — the per-leg values are compared as a sorted pair, not index for index.
//
// Conventions from transaction_v2_handler_integration_test.go apply: NOT parallel (process-global
// huma state), ALLOW_INSECURE_TLS set per test, no time.Now() for business values.

// duplicateLegV1Body / duplicateLegV2Body spell one 1000 USD transaction whose source side names
// @srcA twice — 600 on one leg, 400 on the other — into a single destination taking the full
// 1000. Two legs on one alias is the minimum that distinguishes "each leg resolved" from "one leg
// overwrote the other", because a single leg makes the two indistinguishable. The aliases are the
// ones seedAdvancedLegBalances seeds, so @srcB and @dstB stay named by no leg and a value that
// leaked onto the wrong account shows up as a moved untouched account.
const (
	duplicateLegV1Body = `{
		"description":"duplicate source leg",
		"send":{
			"asset":"USD","value":"1000",
			"source":{"from":[
				{"accountAlias":"@srcA","amount":{"asset":"USD","value":"600"}},
				{"accountAlias":"@srcA","amount":{"asset":"USD","value":"400"}}
			]},
			"distribute":{"to":[{"accountAlias":"@dstA","amount":{"asset":"USD","value":"1000"}}]}
		}
	}`

	duplicateLegV2Body = `{"description":"duplicate source leg","asset":"USD","amount":"1000",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"600"},{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"400"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"1000"}]}`
)

func TestIntegration_TransactionDuplicateSourceLeg_BothLegsSurvive(t *testing.T) {
	// duplicateLegTotal is the declared amount of both bodies above and therefore what each side
	// of the persisted entry must sum to.
	duplicateLegTotal := decimal.NewFromInt(1000)

	// Each surface names the request it posts differently: v1 carries the scope in the URL, v2 in
	// the body. post therefore owns the whole call, so neither surface has to describe the other's
	// spelling.
	cases := []struct {
		name     string
		post     func(*testing.T, *fiber.App, uuid.UUID, uuid.UUID) *nethttp.Response
		buildApp func(*testing.T, *TransactionHandler) *fiber.App
	}{
		{
			name: "v1 detailed",
			post: func(t *testing.T, app *fiber.App, orgID, ledgerID uuid.UUID) *nethttp.Response {
				return postTransaction(t, app, v1JSONURL(orgID, ledgerID), duplicateLegV1Body, "")
			},
			buildApp: func(t *testing.T, h *TransactionHandler) *fiber.App {
				return buildHumaTransactionApp(t, h, true)
			},
		},
		{
			name: "v2 leg arrays",
			post: func(t *testing.T, app *fiber.App, orgID, ledgerID uuid.UUID) *nethttp.Response {
				return postV2Create(t, app, "direct", orgID, ledgerID, duplicateLegV2Body, "")
			},
			buildApp: func(t *testing.T, h *TransactionHandler) *fiber.App {
				return buildHumaV2DirectApp(t, h)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see file header).
			t.Setenv("ALLOW_INSECURE_TLS", "true")

			infra := setupTestInfra(t)
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			ctx := context.Background()

			// @srcA is seeded exactly the total it owes, so a leg that went missing leaves it
			// holding a remainder rather than being absorbed by spare funding.
			balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

			app := tc.buildApp(t, infra.handler)

			result := decodeTxResponse(t, tc.post(t, app, infra.orgID, infra.ledgerID), nethttp.StatusCreated)
			txID := uuid.MustParse(result["id"].(string))

			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
				"a body naming one account twice on one side must settle")

			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

			ops := fetchOperationRows(t, infra.pgContainer.DB, txID)

			// One operation per submitted leg: two on the repeated source, one on the destination.
			// A collapsed leg would show up here as a set of two.
			require.Len(t, ops, 3, "each of the two source legs plus the destination leg must persist its own operation")

			// The two rows of the repeated account carry the two leg values, compared as a sorted
			// pair because their relative order in the persisted set is not defined.
			srcADebits := make([]decimal.Decimal, 0, 2)

			for _, op := range ops {
				if op.AccountAlias == "@srcA" && op.Type == cn.DEBIT {
					srcADebits = append(srcADebits, op.Amount)
				}
			}

			require.Len(t, srcADebits, 2, "the repeated account must be debited once per leg")
			slices.SortFunc(srcADebits, func(a, b decimal.Decimal) int { return a.Cmp(b) })
			requireDecimalEqual(t, decimal.NewFromInt(400), srcADebits[0], "the smaller leg of the repeated account")
			requireDecimalEqual(t, decimal.NewFromInt(600), srcADebits[1], "the larger leg of the repeated account")

			// Double entry holds over the committed rows: both sides sum to the declared total, so
			// no leg was dropped from one side while still counting toward the other.
			assertLegsSumToTotal(t, ops, duplicateLegTotal, tc.name)

			// The repeated account paid the SUM of its legs, the destination received the whole
			// total, and the two accounts no leg names moved by nothing.
			assertAliasBalances(t, infra.pgContainer.DB, advancedLegBalanceIDs(balances), map[string]int64{
				"@srcA": 0,
				"@srcB": 1000,
				"@dstA": 1000,
				"@dstB": 0,
			}, tc.name)
		})
	}
}
