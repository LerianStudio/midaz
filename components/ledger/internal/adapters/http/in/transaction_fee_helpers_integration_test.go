// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	feemodel "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	postgrestestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// ─── HTTP app wiring ─────────────────────────────────────────────────────────

// debugFunnelLogs, when true, injects a Debug-level logger into each request so
// the create funnel's own Debug logs (balance-op counts, etc.) surface in test
// output. Toggled on only while diagnosing.
var debugFunnelLogs = false

// newApp builds a Fiber app exposing the transaction routes the proof suite
// drives, with path-param locals injected exactly as the production
// ProtectedRouteChain does (UUID path params -> Locals). Modes that need a body
// go through http.WithBody so the same decode/validate path as production runs.
func (h *feeHarness) newApp() *fiber.App {
	app := fiber.New()

	paramMiddleware := func(c *fiber.Ctx) error {
		if debugFunnelLogs {
			c.SetUserContext(libObservability.ContextWithLogger(c.UserContext(), &libLog.GoLogger{Level: libLog.LevelDebug}))
		}
		if v := c.Params("organization_id"); v != "" {
			id, _ := uuid.Parse(v)
			c.Locals("organization_id", id)
		}
		if v := c.Params("ledger_id"); v != "" {
			id, _ := uuid.Parse(v)
			c.Locals("ledger_id", id)
		}
		if v := c.Params("transaction_id"); v != "" {
			id, _ := uuid.Parse(v)
			c.Locals("transaction_id", id)
		}
		return c.Next()
	}

	base := "/v1/organizations/:organization_id/ledgers/:ledger_id"

	app.Post(base+"/transactions/json", paramMiddleware,
		http.WithBody(new(mtransaction.CreateTransactionInput), h.handler.CreateTransactionJSON))
	app.Post(base+"/transactions/inflow", paramMiddleware,
		http.WithBody(new(mtransaction.CreateTransactionInflowInput), h.handler.CreateTransactionInflow))
	app.Post(base+"/transactions/outflow", paramMiddleware,
		http.WithBody(new(mtransaction.CreateTransactionOutflowInput), h.handler.CreateTransactionOutflow))
	app.Post(base+"/transactions/annotation", paramMiddleware,
		http.WithBody(new(mtransaction.CreateTransactionInput), h.handler.CreateTransactionAnnotation))
	app.Post(base+"/transactions/dsl", paramMiddleware, h.handler.CreateTransactionDSL)
	app.Post(base+"/transactions/:transaction_id/commit", paramMiddleware, h.handler.CommitTransaction)
	app.Post(base+"/transactions/:transaction_id/cancel", paramMiddleware, h.handler.CancelTransaction)
	app.Post(base+"/transactions/:transaction_id/revert", paramMiddleware, h.handler.RevertTransaction)
	app.Get(base+"/transactions/:transaction_id", paramMiddleware, h.handler.GetTransaction)

	return app
}

// txResponse captures the parsed HTTP response from a create/state call.
type txResponse struct {
	status   int
	rawBody  []byte
	body     map[string]any
	replayed string
}

// post issues a JSON POST and parses the response.
func (h *feeHarness) post(t *testing.T, app *fiber.App, path, body string, headers map[string]string) txResponse {
	t.Helper()

	req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req, -1)
	require.NoError(t, err, "HTTP request failed")

	rb, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	_ = resp.Body.Close()

	out := txResponse{status: resp.StatusCode, rawBody: rb, replayed: resp.Header.Get("X-Idempotency-Replayed")}
	_ = json.Unmarshal(rb, &out.body)

	return out
}

// createJSON drives a transactions/json create and returns the response.
func (h *feeHarness) createJSON(t *testing.T, app *fiber.App, body string, headers map[string]string) txResponse {
	t.Helper()
	return h.post(t, app, h.txPath("json"), body, headers)
}

// txPath builds the create path for a given mode.
func (h *feeHarness) txPath(mode string) string {
	return "/v1/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() + "/transactions/" + mode
}

// statePath builds a commit/cancel/revert path for a transaction.
func (h *feeHarness) statePath(txID uuid.UUID, action string) string {
	return "/v1/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() + "/transactions/" + txID.String() + "/" + action
}

// ─── balance seeding ─────────────────────────────────────────────────────────

// seedBalance creates a balance row and a matching ACTIVE account so the fee
// resolver's GetAccountByAlias finds it. Returns the balance row ID.
func (h *feeHarness) seedBalance(t *testing.T, alias, asset string, available decimal.Decimal, accountType string) uuid.UUID {
	t.Helper()

	accParams := postgrestestutil.DefaultAccountParams()
	accParams.Alias = alias
	accParams.AssetCode = asset
	accParams.Type = accountType
	accountID := postgrestestutil.CreateTestAccountWithParams(t, h.db, h.orgID, h.ledgerID, accParams)

	balParams := postgrestestutil.DefaultBalanceParams()
	balParams.Alias = alias
	balParams.AssetCode = asset
	balParams.Available = available
	balParams.OnHold = decimal.Zero
	balParams.AccountType = accountType

	return postgrestestutil.CreateTestBalance(t, h.db, h.orgID, h.ledgerID, accountID, balParams)
}

// seedBalanceWithSegment is like seedBalance but assigns the account a segment.
func (h *feeHarness) seedBalanceWithSegment(t *testing.T, alias, asset string, available decimal.Decimal, segmentID uuid.UUID) uuid.UUID {
	t.Helper()

	accParams := postgrestestutil.DefaultAccountParams()
	accParams.Alias = alias
	accParams.AssetCode = asset
	accParams.Type = "deposit"
	accParams.SegmentID = &segmentID
	accountID := postgrestestutil.CreateTestAccountWithParams(t, h.db, h.orgID, h.ledgerID, accParams)

	balParams := postgrestestutil.DefaultBalanceParams()
	balParams.Alias = alias
	balParams.AssetCode = asset
	balParams.Available = available
	balParams.AccountType = "deposit"

	return postgrestestutil.CreateTestBalance(t, h.db, h.orgID, h.ledgerID, accountID, balParams)
}

// ─── package seeding ─────────────────────────────────────────────────────────

// feeSpec describes one fee inside a seeded package.
type feeSpec struct {
	label         string
	rule          string // "flatFee" | "percentual" | "maxBetweenTypes"
	calcs         []feemodel.Calculation
	deductible    bool
	creditAccount string
	priority      int
	referenceAmt  string // defaults to originalAmount
}

// packageSpec describes a fee package to seed.
type packageSpec struct {
	label          string
	minAmount      decimal.Decimal
	maxAmount      decimal.Decimal
	segmentID      *uuid.UUID
	waivedAccounts []string
	fees           []feeSpec
}

// seedPackage persists a package from the spec via the real repository and
// returns its ID.
func (h *feeHarness) seedPackage(t *testing.T, spec packageSpec) uuid.UUID {
	t.Helper()

	enable := true
	fees := make(map[string]feemodel.Fee, len(spec.fees))

	for i, f := range spec.fees {
		ded := f.deductible
		ref := f.referenceAmt
		if ref == "" {
			ref = "originalAmount"
		}
		priority := f.priority
		if priority == 0 {
			priority = i + 1
		}

		key := f.label
		if key == "" {
			key = "fee_" + decimal.NewFromInt(int64(i)).String()
		}

		fees[key] = feemodel.Fee{
			FeeLabel: f.label,
			CalculationModel: &feemodel.CalculationModel{
				ApplicationRule: f.rule,
				Calculations:    f.calcs,
			},
			ReferenceAmount:  ref,
			Priority:         priority,
			IsDeductibleFrom: &ded,
			CreditAccount:    f.creditAccount,
		}
	}

	maxAmt := spec.maxAmount
	if maxAmt.IsZero() {
		maxAmt = decimal.NewFromInt(1_000_000_000)
	}

	p, err := pack.NewPackage(h.orgID, h.ledgerID, spec.label, spec.minAmount, maxAmt, fees, &enable)
	require.NoError(t, err, "build package")

	p.SegmentID = spec.segmentID
	if len(spec.waivedAccounts) > 0 {
		wa := spec.waivedAccounts
		p.WaivedAccounts = &wa
	}

	created, err := h.packageRepo.Create(h.ctx(), p, h.orgID)
	require.NoError(t, err, "persist package")

	return created.ID
}

// flatFee builds a flatFee fee spec.
func flatFee(label, creditAccount, value string, deductible bool) feeSpec {
	return feeSpec{
		label:         label,
		rule:          "flatFee",
		calcs:         []feemodel.Calculation{{Type: "flat", Value: value}},
		deductible:    deductible,
		creditAccount: creditAccount,
	}
}

// percentualFee builds a percentual fee spec.
func percentualFee(label, creditAccount, percent string, deductible bool) feeSpec {
	return feeSpec{
		label:         label,
		rule:          "percentual",
		calcs:         []feemodel.Calculation{{Type: "percentage", Value: percent}},
		deductible:    deductible,
		creditAccount: creditAccount,
	}
}

// maxBetweenFee builds a maxBetweenTypes fee spec with a flat and a percentage leg.
func maxBetweenFee(label, creditAccount, flatVal, percentVal string, deductible bool) feeSpec {
	return feeSpec{
		label: label,
		rule:  "maxBetweenTypes",
		calcs: []feemodel.Calculation{
			{Type: "flat", Value: flatVal},
			{Type: "percentage", Value: percentVal},
		},
		deductible:    deductible,
		creditAccount: creditAccount,
	}
}

// ─── persisted operation legs ────────────────────────────────────────────────

// persistedLeg is one row of the Postgres operation table for a transaction.
type persistedLeg struct {
	Type   string
	Alias  string
	Amount decimal.Decimal
	Key    string
	Route  *string
}

// loadLegs reads all operations persisted for a transaction.
func loadLegs(t *testing.T, db *sql.DB, txID uuid.UUID) []persistedLeg {
	t.Helper()

	rows, err := db.Query(`SELECT type, account_alias, amount, balance_key, route FROM operation WHERE transaction_id = $1`, txID)
	require.NoError(t, err, "query operations")
	defer func() { _ = rows.Close() }()

	var legs []persistedLeg
	for rows.Next() {
		var l persistedLeg
		require.NoError(t, rows.Scan(&l.Type, &l.Alias, &l.Amount, &l.Key, &l.Route), "scan operation")
		legs = append(legs, l)
	}
	require.NoError(t, rows.Err(), "operation rows iteration")

	return legs
}

// signedSum computes the signed sum of legs under the double-entry convention:
// CREDIT is positive, DEBIT/ON_HOLD is negative. A balanced transaction nets to
// exactly zero under decimal.Equal.
func signedSum(legs []persistedLeg) decimal.Decimal {
	sum := decimal.Zero
	for _, l := range legs {
		switch l.Type {
		case "CREDIT":
			sum = sum.Add(l.Amount)
		case "DEBIT", "ON_HOLD":
			sum = sum.Sub(l.Amount)
		}
	}
	return sum
}

// requireBalanced asserts the legs net to zero under EXACT decimal equality.
func requireBalanced(t *testing.T, legs []persistedLeg, msg string) {
	t.Helper()
	sum := signedSum(legs)
	require.Truef(t, sum.Equal(decimal.Zero), "%s: legs must net to exactly zero, got %s", msg, sum.String())
}

// feeCreditLegs returns the legs that credit the given fee revenue account.
func feeCreditLegs(legs []persistedLeg, creditAccount string) []persistedLeg {
	var out []persistedLeg
	for _, l := range legs {
		if l.Alias == creditAccount && l.Type == "CREDIT" {
			out = append(out, l)
		}
	}
	return out
}

// sumAmounts sums the absolute amounts of the given legs.
func sumAmounts(legs []persistedLeg) decimal.Decimal {
	sum := decimal.Zero
	for _, l := range legs {
		sum = sum.Add(l.Amount)
	}
	return sum
}

// dbTxStatus reads the persisted transaction status.
func dbTxStatus(t *testing.T, db *sql.DB, txID uuid.UUID) string {
	t.Helper()
	return postgrestestutil.GetTransactionStatus(t, db, txID)
}

// dbTxAmount reads the persisted transaction amount.
func dbTxAmount(t *testing.T, db *sql.DB, txID uuid.UUID) decimal.Decimal {
	t.Helper()
	var amt decimal.Decimal
	err := db.QueryRow(`SELECT amount FROM transaction WHERE id = $1`, txID).Scan(&amt)
	require.NoError(t, err, "read transaction amount")
	return amt
}

// approvedStatus is a convenience alias used across proof assertions.
const approvedStatus = cn.APPROVED
