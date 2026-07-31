// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redisadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// This file is the integration + v1↔v2 parity proof for the v2
// `direct` transaction endpoint (POST /v2/.../transactions/direct). It mounts BOTH the
// v2 `direct` op and the v1 `/json` op through the SAME production Huma seams
// (buildHumaV2DirectApp -> RegisterTransactionV2RoutesToApp and buildHumaTransactionApp
// -> RegisterTransactionRoutes, both defined in the sibling unit-test files) against the
// SAME real-repo handler backed by testcontainers (PostgreSQL + MongoDB + Redis), so
// every assertion exercises the committed money path — not a stub.
//
// NOT PARALLEL: buildHumaTransactionApp / buildHumaV2DirectApp call libProblem.Install()
// (process-global huma.NewError hook) and Huma validation uses process-global sync.Pools;
// concurrent builds cross-contaminate. Every test here stays sequential.
//
// No time.Now() is used for any business value: the v2 Translate sets no transaction date
// (the funnel defaults it), parity ignores IDs/timestamps, and the only wall-clock use is
// the async-idempotency-store poll, which is bounded by a fixed retry count, not a clock.

// --- request helpers ----------------------------------------------------------

// v2DirectURL builds the concrete v2 direct path for the given org/ledger.
func v2DirectURL(orgID, ledgerID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/direct"
}

// v1JSONURL builds the concrete v1 /json path for the given org/ledger.
func v1JSONURL(orgID, ledgerID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/json"
}

// postTransaction issues a POST to the given app/url with the JSON body and an optional
// X-Idempotency header (sent only when idempotencyKey is non-empty).
func postTransaction(t *testing.T, app *fiber.App, url, body, idempotencyKey string) *nethttp.Response {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency", idempotencyKey)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "HTTP request should not fail at the transport layer")

	return resp
}

// decodeTxResponse reads and unmarshals a transaction response body into a generic map,
// asserting the expected status code (dumping the body on mismatch).
func decodeTxResponse(t *testing.T, resp *nethttp.Response, wantStatus int) map[string]any {
	t.Helper()

	body := drainBody(t, resp)

	require.Equal(t, wantStatus, resp.StatusCode,
		"unexpected HTTP status; body: %s", string(body))

	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result), "response should be valid JSON; body: %s", string(body))

	return result
}

// drainBody reads and closes the response body.
func drainBody(t *testing.T, resp *nethttp.Response) []byte {
	t.Helper()

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read response body")

	return body
}

// --- assertion helpers ---------------------------------------------------------

// countTransactionsInLedger counts persisted transactions scoped to a ledger.
func countTransactionsInLedger(t *testing.T, db *sql.DB, ledgerID uuid.UUID) int {
	t.Helper()

	var n int

	err := db.QueryRow(`SELECT COUNT(*) FROM transaction WHERE ledger_id = $1`, ledgerID).Scan(&n)
	require.NoError(t, err, "failed to count transactions in ledger")

	return n
}

// operationEconomicRow is the economically-meaningful projection of an operation row,
// stripped of IDs/timestamps so two operations from different ledgers can be compared for
// v1↔v2 parity.
type operationEconomicRow struct {
	Type           string
	AssetCode      string
	AccountAlias   string
	Amount         decimal.Decimal
	AvailableAfter decimal.Decimal
	OnHoldAfter    decimal.Decimal
}

// fetchOperationRows returns the economic projection of every operation for a transaction,
// ordered by type (CREDIT before DEBIT) for stable cross-ledger comparison.
func fetchOperationRows(t *testing.T, db *sql.DB, txID uuid.UUID) []operationEconomicRow {
	t.Helper()

	rows, err := db.Query(`
		SELECT type, asset_code, account_alias, amount, available_balance_after, on_hold_balance_after
		FROM operation
		WHERE transaction_id = $1
		ORDER BY type
	`, txID)
	require.NoError(t, err, "failed to query operations")

	defer func() { _ = rows.Close() }()

	var out []operationEconomicRow

	for rows.Next() {
		var r operationEconomicRow

		require.NoError(t, rows.Scan(
			&r.Type, &r.AssetCode, &r.AccountAlias, &r.Amount, &r.AvailableAfter, &r.OnHoldAfter,
		), "failed to scan operation row")

		out = append(out, r)
	}

	require.NoError(t, rows.Err(), "operation row iteration error")

	return out
}

// requireProblemCode asserts the RFC 9457 problem body carries EXACTLY the expected
// canonical code. A substring check over the raw body would also match the code appearing
// inside `type` or a message, so the field is read out of the parsed envelope.
func requireProblemCode(t *testing.T, body []byte, wantCode string) {
	t.Helper()

	var problem map[string]any
	require.NoError(t, json.Unmarshal(body, &problem), "an error response must be a JSON problem envelope; body: %s", string(body))

	code, ok := problem["code"].(string)
	require.Truef(t, ok, "the problem envelope must carry a string code; body: %s", string(body))

	require.Equal(t, wantCode, code, "the rejection must come from the expected layer; body: %s", string(body))
}

// requireDecimalEqual asserts two decimals are numerically equal (no float comparison).
func requireDecimalEqual(t *testing.T, want, got decimal.Decimal, msgAndArgs ...any) {
	t.Helper()

	require.Truef(t, want.Equal(got), "expected decimal %s, got %s (%v)", want.String(), got.String(), msgAndArgs)
}

// volatileResponseKeys are the identity/timestamp fields deleted (recursively) before a
// v1↔v2 response deep-equal, so two economically-identical transactions in two ledgers
// compare equal on everything that carries economic meaning.
var volatileResponseKeys = map[string]struct{}{
	"id":                  {},
	"transactionId":       {},
	"parentTransactionId": {},
	"ledgerId":            {},
	"organizationId":      {},
	"accountId":           {},
	"balanceId":           {},
	"createdAt":           {},
	"updatedAt":           {},
	"deletedAt":           {},
	"route":               {},
	"routeId":             {},
}

// stripVolatile recursively removes identity/timestamp keys so the remaining tree is the
// economic content of the transaction response.
//
// `route` / `routeId` are in that stripped set, so route linkage is deliberately OUTSIDE the
// economic-parity envelope: a deep-equal over the result proves nothing about whether two
// surfaces resolve the same operation/transaction route. v1↔v2 route-linkage parity is
// therefore UNCOVERED by this suite. The nearest case — the revert bidirectional-route subject
// in the lifecycle file — asserts REJECTION (422 on a non-bidirectional route), not that a
// route-stamped origin resolves to the same routeId on both surfaces.
func stripVolatile(v any) any {
	switch node := v.(type) {
	case map[string]any:
		for k := range node {
			if _, drop := volatileResponseKeys[k]; drop {
				delete(node, k)
				continue
			}

			node[k] = stripVolatile(node[k])
		}

		return node
	case []any:
		for i := range node {
			node[i] = stripVolatile(node[i])
		}

		return node
	default:
		return v
	}
}

// waitForIdempotencyStored polls Redis until the async goroutine
// (SetTransactionIdempotencyValue, fired post-write in executeCreateTransaction) has stored
// the serialized transaction under the idempotency slot. SetNX seeds the slot with an EMPTY
// value first; the cached response lands only when the goroutine runs, so a replay call
// issued too early would race into the "key in use" 409 instead of the cached replay. The
// bound is a fixed retry count (no wall clock).
func waitForIdempotencyStored(t *testing.T, ctx context.Context, redisRepo redisadapter.RedisRepository, orgID, ledgerID uuid.UUID, key string) {
	t.Helper()

	internalKey := utils.IdempotencyInternalKey(orgID, ledgerID, key)

	for i := 0; i < 400; i++ {
		value, err := redisRepo.Get(ctx, internalKey)
		if err == nil && strings.HasPrefix(strings.TrimSpace(value), "{") {
			return
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("idempotency cached response was not stored in redis for key %q within the retry budget", key)
}

// seedTransfer seeds a source/destination balance pair for a transfer test and returns
// their balance IDs. Source starts with `available` and zero on-hold; destination starts
// empty. Aliases are fixed so identical aliases can be reused across ledgers for parity.
func seedTransfer(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, sourceAlias, destAlias string, available int64) (sourceBalanceID, destBalanceID uuid.UUID) {
	t.Helper()

	return seedFundedTransfer(t, db, orgID, ledgerID, sourceAlias, destAlias, available, 0)
}

// seedFundedTransfer is seedTransfer with an explicitly funded DESTINATION. An empty
// destination silently caps how much can flow back out of it, which turns the balance layer
// into an accidental backstop for tests about duplicate reversals; giving the destination
// headroom keeps those tests sensitive to the mechanism they actually assert.
func seedFundedTransfer(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, sourceAlias, destAlias string, sourceAvailable, destAvailable int64) (sourceBalanceID, destBalanceID uuid.UUID) {
	t.Helper()

	sourceAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	destAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	srcParams := postgrestestutil.DefaultBalanceParams()
	srcParams.Alias = sourceAlias
	srcParams.AssetCode = "USD"
	srcParams.Available = decimal.NewFromInt(sourceAvailable)
	srcParams.OnHold = decimal.Zero
	sourceBalanceID = postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, sourceAccountID, srcParams)

	dstParams := postgrestestutil.DefaultBalanceParams()
	dstParams.Alias = destAlias
	dstParams.AssetCode = "USD"
	dstParams.Available = decimal.NewFromInt(destAvailable)
	dstParams.OnHold = decimal.Zero
	destBalanceID = postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, destAccountID, dstParams)

	return sourceBalanceID, destBalanceID
}

// equivalentV2Body / equivalentV1Body are the economically-identical 100 USD transfer
// bodies for the two surfaces, using the same aliases so the resulting transactions differ
// only by IDs/timestamps.
const (
	equivalentV2Body = `{"description":"v1 v2 parity transfer","asset":"USD","amount":"100","from":"@src","to":"@dst"}`

	equivalentV1Body = `{
		"description":"v1 v2 parity transfer",
		"send":{
			"asset":"USD",
			"value":"100",
			"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},
			"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}
		}
	}`
)

// =============================================================================
// 1. PARITY (core): v2 `direct` is indistinguishable from v1 `/json` for the same
//    economic intent — same asset, amount, legs, source/destination effects, and final
//    Available/OnHold on both accounts, comparing transactions/operations ignoring
//    IDs and timestamps.
// =============================================================================

func TestIntegration_TransactionV2Direct_ParityWithV1JSON(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and
	// refuses plaintext unless ALLOW_INSECURE_TLS=true. `make test-integration` exports it;
	// set it here too so the test is runnable directly (mirrors
	// composition_mt_isolation_integration_test.go).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org so both transfers can use IDENTICAL aliases and
	// starting balances; the only legitimate difference in the two responses is then the
	// ledger id (stripped) plus per-row IDs/timestamps.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Act: economically-equivalent transfers on each surface. Each surface is fully
	// processed (create -> drain -> assert balances) BEFORE the next is created, because
	// the balance-sync schedule (schedule:balance-sync ZSET) is GLOBAL, not per-ledger:
	// draining ledgerV1 while ledgerV2's keys are still pending would claim them under the
	// wrong ledger and leave ledgerV2's cold balances stale.
	v1Resp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), equivalentV1Body, ""), nethttp.StatusCreated)
	v1TxID := uuid.MustParse(v1Resp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be APPROVED in DB")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v2Resp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, ledgerV2), equivalentV2Body, ""), nethttp.StatusCreated)
	v2TxID := uuid.MustParse(v2Resp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be APPROVED in DB")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Balances: source 1000 -> 900, destination 0 -> 100, on-hold 0, on BOTH surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available")
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest available")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest available")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold")

	// Operations: exactly 1 DEBIT + 1 CREDIT on each, and the economic projection is
	// IDENTICAL between the two surfaces (type, asset, alias, amount, balance-after).
	v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
	v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)

	require.Len(t, v1Ops, 2, "v1 transaction should have exactly 2 operations")
	require.Len(t, v2Ops, 2, "v2 transaction should have exactly 2 operations")
	assertOperationSetsEqual(t, v1Ops, v2Ops)

	// Response deep-equal, ignoring IDs/timestamps: the v2 direct response is
	// indistinguishable from the v1 /json response for the same economic intent.
	assert.Equal(t, "USD", v1Resp["assetCode"])
	assert.Equal(t, "USD", v2Resp["assetCode"])

	require.Equal(t, stripVolatile(v1Resp), stripVolatile(v2Resp),
		"v2 direct transaction must be indistinguishable from the v1 /json equivalent (ignoring IDs/timestamps)")
}

// assertOperationSetsEqual asserts two 2-element operation sets carry identical economic
// content leg-for-leg. Both inputs come from fetchOperationRows, whose `ORDER BY type` is
// the single source of ordering, so the rows line up index-for-index without re-sorting.
func assertOperationSetsEqual(t *testing.T, a, b []operationEconomicRow) {
	t.Helper()

	require.Equal(t, len(a), len(b), "operation set sizes differ")

	for i := range a {
		assert.Equal(t, a[i].Type, b[i].Type, "operation[%d] type", i)
		assert.Equal(t, a[i].AssetCode, b[i].AssetCode, "operation[%d] asset", i)
		assert.Equal(t, a[i].AccountAlias, b[i].AccountAlias, "operation[%d] alias", i)
		requireDecimalEqual(t, a[i].Amount, b[i].Amount, "operation[%d] amount", i)
		requireDecimalEqual(t, a[i].AvailableAfter, b[i].AvailableAfter, "operation[%d] available-after", i)
		requireDecimalEqual(t, a[i].OnHoldAfter, b[i].OnHoldAfter, "operation[%d] on-hold-after", i)
	}
}

// =============================================================================
// 2. VALIDATION BEFORE ANY LEDGER EFFECT: a v2 `direct` missing a required field (or with
//    a malformed body) is rejected at the decode boundary (4xx) with NO transaction /
//    operation / balance mutation.
// =============================================================================

func TestIntegration_TransactionV2Direct_ValidationBeforeLedgerEffect(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and
	// refuses plaintext unless ALLOW_INSECURE_TLS=true. `make test-integration` exports it;
	// set it here too so the test is runnable directly (mirrors
	// composition_mt_isolation_integration_test.go).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)
	url := v2DirectURL(infra.orgID, infra.ledgerID)

	// The table deliberately spans FOUR rejection layers, and the canonical code plus the
	// layer-specific text in the body is the only thing that identifies which one answered.
	// Asserting status alone would let a row pass on a 400 raised anywhere — including a
	// layer that has no business seeing the body at all.
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
		// wantBodyContains distinguishes the two layers that both answer with 0009: the
		// Translate side rule names the field PAIR in its detail, while the struct-tag
		// layer reports the offending field (leg index included) in the errors array.
		wantBodyContains string
	}{
		{
			// Each side is spelled EITHER scalar (`from`) or as a leg array
			// (`sources`), so requiring a side is a request-shape rule across a pair of
			// fields, not a per-field one. No struct tag expresses that, which is why —
			// unlike the `asset` case below — this body is not rejected by
			// DecodeAndValidate's struct validation. Translate owns the rule instead and
			// rejects it with ErrMissingFieldsInRequest (0009) -> ValidationError -> 400.
			name:             "missing from field",
			body:             `{"asset":"USD","amount":"100","to":"@dst"}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0009",
			wantBodyContains: "from or sources",
		},
		{
			// The struct-tag `required` layer. It answers with the SAME 0009 the Translate
			// side rule uses, so only the per-field entry in the errors array tells the two
			// apart — which is why this row pins that text and the row above pins the pair.
			name:             "missing required asset field",
			body:             `{"amount":"100","from":"@src","to":"@dst"}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0009",
			wantBodyContains: "asset is a required field",
		},
		{
			// The JSON decode layer, upstream of every validator: 0094.
			name:       "malformed json body",
			body:       `{not-json`,
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   "0094",
		},
		{
			// A leg naming no account. The obligation is a struct tag rather than a
			// Translate check precisely so the rejection names the offending leg by INDEX;
			// pinning that text is what keeps the index in the contract.
			name:             "leg without an account",
			body:             `{"asset":"USD","amount":"100","sources":[{"amount":"100"}],"to":"@dst"}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0009",
			wantBodyContains: "sources[0].account",
		},
		{
			// Both spellings on ONE side is the mutual-exclusivity violation (0498). The
			// destination side is used because per-side exclusivity makes the mixed shape
			// (scalar source + array destinations) legal, so only a side spelled twice is
			// a violation.
			name:       "destination side spelled both scalar and as a leg array",
			body:       `{"asset":"USD","amount":"100","from":"@src","to":"@dst","destinations":[{"account":"@dst","amount":"100"}]}`,
			wantStatus: nethttp.StatusBadRequest,
			wantCode:   "0498",
		},
		{
			// A leg filling NEITHER value expression: 0072, naming the offending side AND
			// the offending leg's index. The leg here is the SECOND one, so a message that
			// hardcoded index 0 fails this row.
			name:             "leg with no value expression",
			body:             `{"asset":"USD","amount":"100","sources":[{"account":"@srcA","amount":"100"},{"account":"@srcB"}],"to":"@dst"}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0072",
			wantBodyContains: "'sources[1]'",
		},
		{
			// A leg whose explicit amount is zero. Unlike the four rows above this is a
			// VALUE rule, not a shape rule, so it is a 422 rather than a 400 — the status
			// and the code move together and both are pinned.
			name:       "leg with a zero amount",
			body:       `{"asset":"USD","amount":"100","sources":[{"account":"@srcA","amount":"0"}],"to":"@dst"}`,
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   "0125",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postTransaction(t, v2App, url, tc.body, "")
			body := drainBody(t, resp)

			assert.Equal(t, tc.wantStatus, resp.StatusCode, "%s should be rejected before any ledger effect; body: %s", tc.name, string(body))
			requireProblemCode(t, body, tc.wantCode)

			if tc.wantBodyContains != "" {
				assert.Contains(t, string(body), tc.wantBodyContains,
					"the rejection must name the offending field, which is what identifies the answering layer")
			}
		})
	}

	// No ledger effect: no transaction, and balances untouched.
	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "no transaction should be persisted for rejected requests")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source balance must be untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination balance must be untouched")
}

// =============================================================================
// 3. BUSINESS RULE: `from == to` is a Translate business error -> 422, with NO ledger
//    effect (the error fires before the create funnel touches any balance).
// =============================================================================

func TestIntegration_TransactionV2Direct_FromEqualsTo_BusinessError(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and
	// refuses plaintext unless ALLOW_INSECURE_TLS=true. `make test-integration` exports it;
	// set it here too so the test is runnable directly (mirrors
	// composition_mt_isolation_integration_test.go).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	srcID, _ := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@same", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	resp := postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), `{"asset":"USD","amount":"100","from":"@same","to":"@same"}`, "")
	body := drainBody(t, resp)

	assert.Equal(t, nethttp.StatusUnprocessableEntity, resp.StatusCode,
		"from == to is a business error (ErrTransactionAmbiguous / 0090) -> 422; body: %s", string(body))
	assert.Contains(t, string(body), "0090", "response should carry the ambiguous-transaction business error code")

	// No ledger effect.
	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "ambiguous transaction must not persist")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "balance must be untouched on business error")
}

// =============================================================================
// 4. IDEMPOTENCY:
//    (a) two identical v2 `direct` calls (same X-Idempotency key + body) -> the second
//        REPLAYS the first (X-Idempotency-Replayed: true, same tx id) and creates NO
//        second transaction.
//    (b) a v1 `/json` call keyed off the SAME economic intent does NOT cross-dedup with
//        the v2 one: v2 keys idempotency off the RAW v2 body, v1 off the CANONICAL built
//        transaction, so the hashes differ by construction and each surface creates its
//        own distinct transaction.
// =============================================================================

func TestIntegration_TransactionV2Direct_Idempotency(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and
	// refuses plaintext unless ALLOW_INSECURE_TLS=true. `make test-integration` exports it;
	// set it here too so the test is runnable directly (mirrors
	// composition_mt_isolation_integration_test.go).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// --- (a) v2 replay on an explicit idempotency key -----------------------------
	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	idempotencyKey := uuid.NewString()
	url := v2DirectURL(infra.orgID, infra.ledgerID)

	first := postTransaction(t, v2App, url, equivalentV2Body, idempotencyKey)
	firstResult := decodeTxResponse(t, first, nethttp.StatusCreated)
	assert.Equal(t, "false", first.Header.Get("X-Idempotency-Replayed"), "first call must not be a replay")

	firstTxID := firstResult["id"].(string)

	// Wait for the async idempotency-value store before replaying (see helper doc).
	waitForIdempotencyStored(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, idempotencyKey)

	second := postTransaction(t, v2App, url, equivalentV2Body, idempotencyKey)
	secondResult := decodeTxResponse(t, second, nethttp.StatusCreated)

	assert.Equal(t, "true", second.Header.Get("X-Idempotency-Replayed"), "second identical call must be a replay")
	assert.Equal(t, firstTxID, secondResult["id"].(string), "replay must return the FIRST transaction's id")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"an idempotent replay must NOT create a second transaction")

	// --- (b) v1 does NOT cross-dedup with v2 (different key source) -------
	// Both calls below omit X-Idempotency, so each surface derives its key from the body
	// hash: v2 from the raw flat body, v1 from the canonical built transaction. The hashes
	// differ by construction, so neither replays the other. Distinct account aliases keep
	// the balance effects independent; the idempotency internal key is (org, ledger, hash)
	// and does not include the accounts, so the non-collision is proven purely by the hash
	// source difference within the SAME ledger.
	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@xsrc", "@xdst", 1000)
	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@ysrc", "@ydst", 1000)

	v2CrossBody := `{"description":"cross dedup","asset":"USD","amount":"100","from":"@xsrc","to":"@xdst"}`
	v1CrossBody := `{
		"description":"cross dedup",
		"send":{
			"asset":"USD",
			"value":"100",
			"source":{"from":[{"accountAlias":"@ysrc","amount":{"asset":"USD","value":"100"}}]},
			"distribute":{"to":[{"accountAlias":"@ydst","amount":{"asset":"USD","value":"100"}}]}
		}
	}`

	countBefore := countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID)

	v2Cross := postTransaction(t, v2App, url, v2CrossBody, "")
	v2CrossResult := decodeTxResponse(t, v2Cross, nethttp.StatusCreated)

	v1Cross := postTransaction(t, v1App, v1JSONURL(infra.orgID, infra.ledgerID), v1CrossBody, "")
	v1CrossResult := decodeTxResponse(t, v1Cross, nethttp.StatusCreated)

	assert.Equal(t, "false", v1Cross.Header.Get("X-Idempotency-Replayed"),
		"the v1 call must NOT replay the v2 transaction (different idempotency key source)")
	assert.NotEqual(t, v2CrossResult["id"].(string), v1CrossResult["id"].(string),
		"v1 and v2 body-hash keys must not collide: each surface creates its own transaction")
	assert.Equal(t, countBefore+2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"v2 and v1 must each create a distinct transaction (no cross-dedup)")
}

// =============================================================================
// 5. INSUFFICIENT FUNDS (money-path defense-in-depth): a v2 `direct` transfer for MORE
//    than the source's available balance is rejected by the atomic balance commit as a
//    business error (ErrInsufficientFunds / 0018 -> 422), with NO transaction persisted
//    and BOTH balances left exactly as seeded (the Lua commit is atomic, so a rejected
//    transfer moves nothing).
// =============================================================================

func TestIntegration_TransactionV2Direct_InsufficientFunds(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and
	// refuses plaintext unless ALLOW_INSECURE_TLS=true. `make test-integration` exports it;
	// set it here too so the test is runnable directly (mirrors
	// composition_mt_isolation_integration_test.go).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	// Source seeded with 1000 available; the transfer below asks for 5000.
	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	resp := postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID),
		`{"description":"v2 insufficient funds","asset":"USD","amount":"5000","from":"@src","to":"@dst"}`, "")
	body := drainBody(t, resp)

	assert.Equal(t, nethttp.StatusUnprocessableEntity, resp.StatusCode,
		"a transfer exceeding available funds is a business error (ErrInsufficientFunds / 0018) -> 422; body: %s", string(body))
	assert.Contains(t, string(body), "0018", "response should carry the insufficient-funds business error code")

	// No ledger effect: no transaction persisted, and both balances untouched (available
	// and on-hold), because the atomic commit rejected before moving anything.
	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"an insufficient-funds transfer must not persist a transaction")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available must be untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold must be untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination available must be untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "destination on-hold must be untouched")
}

// v2HoldURL builds the concrete v2 hold path for the given org/ledger. The hold op is
// mounted by the SAME RegisterTransactionV2RoutesToApp seam as direct, so the v2 app
// built by buildHumaV2DirectApp serves both.
func v2HoldURL(orgID, ledgerID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/hold"
}

// v1CommitURL builds the concrete v1 commit path for a pending transaction. It is used
// deliberately against transactions held through v2, to prove the two surfaces settle the
// same hold — not because v2 lacks a commit op of its own.
func v1CommitURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String() + "/commit"
}

// holdParityV2Body / holdParityV1PendingBody are the economically-identical 100 USD hold
// bodies for the two surfaces, using the same aliases and description so the resulting
// PENDING transactions differ only by IDs/timestamps. The v2 flat `hold` action carries
// its pending intent in the endpoint; the v1 `/json` action carries it in `pending:true`.
const (
	holdParityV2Body = `{"description":"v1 v2 hold parity transfer","asset":"USD","amount":"100","from":"@src","to":"@dst"}`

	holdParityV1PendingBody = `{
		"description":"v1 v2 hold parity transfer",
		"pending":true,
		"send":{
			"asset":"USD",
			"value":"100",
			"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},
			"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}
		}
	}`
)

// =============================================================================
// 6. HOLD PARITY (core): v2 `hold` is indistinguishable from a v1 `/json` with
//    `pending:true` for the same economic intent — both open a PENDING transaction
//    that reserves the source (available down, on-hold up) and leaves the destination
//    untouched, producing a single ON_HOLD operation, comparing transactions/operations
//    ignoring IDs and timestamps.
// =============================================================================

func TestIntegration_TransactionV2Hold_ParityWithV1PendingJSON(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and
	// refuses plaintext unless ALLOW_INSECURE_TLS=true. `make test-integration` exports it;
	// set it here too so the test is runnable directly.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under the SAME org so both holds can use IDENTICAL aliases and starting
	// balances; the only legitimate difference in the two responses is then the ledger id
	// (stripped) plus per-row IDs/timestamps.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
	v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Act: economically-equivalent holds on each surface. Each surface is fully processed
	// (create -> drain -> assert) BEFORE the next, because the balance-sync schedule ZSET is
	// GLOBAL, not per-ledger (see the direct parity test for the same discipline).
	v1Resp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), holdParityV1PendingBody, ""), nethttp.StatusCreated)
	v1TxID := uuid.MustParse(v1Resp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 pending transaction should be PENDING in DB")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

	v2Resp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, ledgerV2), holdParityV2Body, ""), nethttp.StatusCreated)
	v2TxID := uuid.MustParse(v2Resp["id"].(string))
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 hold transaction should be PENDING in DB")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

	// Balances after hold: source 1000 -> available 900 / on-hold 100 (funds reserved),
	// destination untouched (0 / 0, credit not applied until commit), on BOTH surfaces.
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available after hold")
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available after hold")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold after hold")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold after hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest available untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest available untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Dst), "v1 dest on-hold untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Dst), "v2 dest on-hold untouched")

	// Operations: exactly 1 ON_HOLD (source only) on each, and the economic projection is
	// IDENTICAL between the two surfaces (type, asset, alias, amount, balance-after).
	v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
	v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)

	require.Len(t, v1Ops, 1, "v1 pending transaction should have exactly 1 operation (source hold)")
	require.Len(t, v2Ops, 1, "v2 hold transaction should have exactly 1 operation (source hold)")
	assertOperationSetsEqual(t, v1Ops, v2Ops)

	// Response deep-equal, ignoring IDs/timestamps: the v2 hold response is indistinguishable
	// from the v1 `/json` pending response for the same economic intent.
	assert.Equal(t, "USD", v1Resp["assetCode"])
	assert.Equal(t, "USD", v2Resp["assetCode"])

	require.Equal(t, stripVolatile(v1Resp), stripVolatile(v2Resp),
		"v2 hold transaction must be indistinguishable from the v1 /json pending equivalent (ignoring IDs/timestamps)")
}

// =============================================================================
// 7. HOLD COMMIT (lifecycle): a v2-held transaction is committable through the v1 commit
//    endpoint, proving the hold is settleable from either surface. Commit settles the hold:
//    the source on-hold releases and the destination credit applies, the transaction flips
//    to APPROVED, and the full ON_HOLD + DEBIT + CREDIT operation set is present.
// =============================================================================

func TestIntegration_TransactionV2Hold_CommitSettles(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Create the hold on the v2 surface.
	holdResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, infra.ledgerID), holdParityV2Body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(holdResp["id"].(string))

	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID), "v2 hold should open the transaction as PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// After hold: source reserved (900 / 100), destination untouched (0 / 0).
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available after hold")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold after hold")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "dest available before commit")
	assert.Equal(t, 1, postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID), "hold should create exactly 1 ON_HOLD operation")

	// Commit through the existing v1 endpoint; the v2-held transaction shares the same
	// handler/use-cases/DB, so it settles exactly like a v1 pending transaction.
	_ = decodeTxResponse(t, postTransaction(t, infra.app, v1CommitURL(infra.orgID, infra.ledgerID, txID), "", ""), nethttp.StatusCreated)

	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID), "transaction should be APPROVED after commit")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// After commit: source on-hold released (available stays 900, on-hold 0), destination
	// credited (available 100).
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, srcID), "source on-hold released after commit")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "dest credited after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, dstID), "dest on-hold after commit")

	// Full lifecycle operation TYPE set: 1 ON_HOLD (hold) + 1 DEBIT (release) + 1 CREDIT
	// (apply). Asserting the set of types (not merely count == 3) proves the three distinct
	// lifecycle legs are present, so a regression that emitted three operations of the wrong
	// type mix would still be caught. fetchOperationRows is the same economic projection the
	// parity tests use.
	commitOps := fetchOperationRows(t, infra.pgContainer.DB, txID)

	commitOpTypes := make([]string, 0, len(commitOps))
	for _, op := range commitOps {
		commitOpTypes = append(commitOpTypes, op.Type)
	}

	assert.ElementsMatch(t, []string{cn.ONHOLD, cn.DEBIT, cn.CREDIT}, commitOpTypes,
		"committed hold should carry exactly the ON_HOLD + DEBIT + CREDIT operation set")
}

// =============================================================================
// 8. HOLD IDEMPOTENCY: two identical v2 `hold` calls (same X-Idempotency key + body)
//    -> the second REPLAYS the first (X-Idempotency-Replayed: true, same tx id) and
//    creates NO second transaction, identical to the v2 `direct` idempotency seam
//    (both key off the raw v2 body).
// =============================================================================

func TestIntegration_TransactionV2Hold_Idempotency(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)
	idempotencyKey := uuid.NewString()
	url := v2HoldURL(infra.orgID, infra.ledgerID)

	first := postTransaction(t, v2App, url, holdParityV2Body, idempotencyKey)
	firstResult := decodeTxResponse(t, first, nethttp.StatusCreated)
	assert.Equal(t, "false", first.Header.Get("X-Idempotency-Replayed"), "first hold call must not be a replay")

	firstTxID := firstResult["id"].(string)

	// Wait for the async idempotency-value store before replaying (see helper doc).
	waitForIdempotencyStored(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, idempotencyKey)

	second := postTransaction(t, v2App, url, holdParityV2Body, idempotencyKey)
	secondResult := decodeTxResponse(t, second, nethttp.StatusCreated)

	assert.Equal(t, "true", second.Header.Get("X-Idempotency-Replayed"), "second identical hold call must be a replay")
	assert.Equal(t, firstTxID, secondResult["id"].(string), "replay must return the FIRST hold transaction's id")
	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"an idempotent hold replay must NOT create a second transaction")
}

// =============================================================================
// 9. DIRECT↔HOLD NO-KEY CROSS-DEDUP: the v2 action (direct vs hold) is carried by the
//    ENDPOINT, not the body, so a byte-identical flat body posted to /direct and then to
//    /hold in the SAME org/ledger with NO X-Idempotency header must NOT cross-replay. Each
//    action folds its own identity into the idempotency hash source (direct = bare body,
//    hold = discriminated body), so the two claims land in distinct slots -> two DISTINCT
//    transactions with distinct statuses (direct APPROVED, hold PENDING), never a replay.
// =============================================================================

func TestIntegration_TransactionV2_DirectHoldNoKeyCrossDedup(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	// Source seeded with 1000: enough to cover the direct transfer (100) AND the subsequent
	// hold reservation (100) so both actions clear the funds guard and reach persistence.
	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// Byte-identical flat body; the ONLY difference between the two POSTs is the endpoint.
	// No X-Idempotency header, so each surface derives its key from the (discriminated) body
	// hash — the exact collision path that cross-dedup would exploit.
	body := `{"description":"direct hold cross dedup","asset":"USD","amount":"100","from":"@src","to":"@dst"}`

	directResp := postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), body, "")
	directResult := decodeTxResponse(t, directResp, nethttp.StatusCreated)
	assert.Equal(t, "false", directResp.Header.Get("X-Idempotency-Replayed"), "direct create must not be a replay")

	directTxID := uuid.MustParse(directResult["id"].(string))

	holdResp := postTransaction(t, v2App, v2HoldURL(infra.orgID, infra.ledgerID), body, "")
	holdResult := decodeTxResponse(t, holdResp, nethttp.StatusCreated)
	assert.Equal(t, "false", holdResp.Header.Get("X-Idempotency-Replayed"),
		"the hold create must NOT replay the direct transaction (distinct action identity, distinct idempotency slot)")

	holdTxID := uuid.MustParse(holdResult["id"].(string))

	// Two DISTINCT transaction IDs — the hold did not replay the direct.
	assert.NotEqual(t, directTxID, holdTxID,
		"identical bodies to /direct and /hold must create two DISTINCT transactions (no cross-dedup)")

	// Distinct statuses: the direct settles immediately (APPROVED), the hold stays PENDING.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, directTxID),
		"the direct transaction should be APPROVED")
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, holdTxID),
		"the hold transaction should be PENDING")

	// Exactly two persisted transactions — no replay collapsed them into one.
	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"direct + hold with an identical no-key body must persist two transactions, not replay one")
}

// v2BlockURL / v2UnblockURL build the concrete v2 block/unblock paths. Both ops are mounted
// by the SAME RegisterTransactionV2RoutesToApp seam as direct/hold, so the v2 app built by
// buildHumaV2DirectApp serves all four.
func v2BlockURL(orgID, ledgerID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/block"
}

func v2UnblockURL(orgID, ledgerID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/unblock"
}

// v1BlockURL / v1UnblockURL build the concrete v1 block/unblock paths. The v1 block/unblock
// Huma ops are registered by RegisterTransactionRoutes (inside buildHumaTransactionApp) and
// enter the SAME createTransactionShell funnel the v2 actions do — the funnel parses org/ledger
// from the path string, so these are the parity reference for the v2 block/unblock actions.
func v1BlockURL(orgID, ledgerID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/block"
}

func v1UnblockURL(orgID, ledgerID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/unblock"
}

// indexOpsByAlias keys a 2-leg operation set by its (unique) account alias. Both BLOCK legs
// carry the SAME Type, so `fetchOperationRows`' `ORDER BY type` cannot line up the two legs
// index-for-index the way it does for a DEBIT/CREDIT direct set; keying by the aliases (which
// ARE distinct and shared across the two surfaces) is the stable join for a cross-surface
// leg-for-leg comparison.
func indexOpsByAlias(t *testing.T, ops []operationEconomicRow) map[string]operationEconomicRow {
	t.Helper()

	out := make(map[string]operationEconomicRow, len(ops))

	for _, op := range ops {
		_, dup := out[op.AccountAlias]
		require.Falsef(t, dup, "duplicate operation alias %s breaks the alias join", op.AccountAlias)
		out[op.AccountAlias] = op
	}

	return out
}

// assertBlockOpsParity asserts the two block/unblock operation sets carry identical economic
// content leg-for-leg (joined by alias) AND that every persisted leg carries the expected
// BLOCK/UNBLOCK Type — the observable marker of the OperationTypeOverride. The accounting
// content (asset, amount, balance-after) must be indistinguishable between the v1 and v2
// surfaces, proving the override relabels Type WITHOUT touching direction/value/balance.
func assertBlockOpsParity(t *testing.T, v1Ops, v2Ops []operationEconomicRow, expectedType string) {
	t.Helper()

	require.Len(t, v1Ops, 2, "v1 block/unblock transaction should have exactly 2 operations")
	require.Len(t, v2Ops, 2, "v2 block/unblock transaction should have exactly 2 operations")

	v1ByAlias := indexOpsByAlias(t, v1Ops)
	v2ByAlias := indexOpsByAlias(t, v2Ops)

	for alias, v1op := range v1ByAlias {
		v2op, ok := v2ByAlias[alias]
		require.Truef(t, ok, "v2 set is missing the operation for alias %s", alias)

		assert.Equal(t, expectedType, v1op.Type, "v1 operation[%s] must carry Type=%s", alias, expectedType)
		assert.Equal(t, expectedType, v2op.Type, "v2 operation[%s] must carry Type=%s", alias, expectedType)
		assert.Equal(t, v1op.AssetCode, v2op.AssetCode, "operation[%s] asset", alias)
		requireDecimalEqual(t, v1op.Amount, v2op.Amount, "operation[%s] amount", alias)
		requireDecimalEqual(t, v1op.AvailableAfter, v2op.AvailableAfter, "operation[%s] available-after", alias)
		requireDecimalEqual(t, v1op.OnHoldAfter, v2op.OnHoldAfter, "operation[%s] on-hold-after", alias)
	}
}

// fetchBalanceFlags reads the account-level block flags (allow_sending / allow_receiving) for
// a balance. A transaction-level BLOCK/UNBLOCK must leave these untouched: the override is a
// per-operation Type label, not an account-block state change.
func fetchBalanceFlags(t *testing.T, db *sql.DB, balanceID uuid.UUID) (allowSending, allowReceiving bool) {
	t.Helper()

	err := db.QueryRow(
		`SELECT allow_sending, allow_receiving FROM balance WHERE id = $1`, balanceID,
	).Scan(&allowSending, &allowReceiving)
	require.NoError(t, err, "failed to read balance flags")

	return allowSending, allowReceiving
}

// assertReasonMetadata asserts the create response carries the block/unblock reason under the
// flat metadata key. The v2 flat body and the v1 send/distribute body both surface metadata
// on the created transaction, so the reason is preserved identically on either surface.
func assertReasonMetadata(t *testing.T, resp map[string]any, reason string) {
	t.Helper()

	md, ok := resp["metadata"].(map[string]any)
	require.Truef(t, ok, "create response should carry a metadata object; got %T", resp["metadata"])
	assert.Equal(t, reason, md["reason"], "metadata.reason should carry the block/unblock reason")
}

// =============================================================================
// 10. BLOCK / UNBLOCK PARITY (core): the v2 `block` / `unblock` actions are indistinguishable
//     from their v1 endpoints for the same economic intent. Both stamp the BLOCK/UNBLOCK
//     OperationTypeOverride, which relabels the persisted Operation.Type WITHOUT changing
//     accounting direction/value/balance, keeps the transaction non-pending (settles like a
//     direct transfer), carries the reason through metadata, and — with no Block/Unblock
//     AccountingEntry configured on the (routeless, default-settings) ledger — resolves the
//     rubric via the Direct fallback with NO error. Parity is asserted leg-for-leg on the
//     persisted operations, on the final balances, and by a full response deep-equal
//     (ignoring IDs/timestamps).
// =============================================================================

func TestIntegration_TransactionV2BlockUnblock_ParityWithV1(t *testing.T) {
	cases := []struct {
		name         string
		expectedType string
		reason       string
		v2URL        func(orgID, ledgerID uuid.UUID) string
		v1URL        func(orgID, ledgerID uuid.UUID) string
		v2Body       string
		v1Body       string
	}{
		{
			name:         "block v2 matches v1 block ledger effect",
			expectedType: cn.BLOCK,
			reason:       "regulatory-hold",
			v2URL:        v2BlockURL,
			v1URL:        v1BlockURL,
			v2Body:       `{"description":"v1 v2 block parity transfer","asset":"USD","amount":"100","from":"@src","to":"@dst","metadata":{"reason":"regulatory-hold"}}`,
			v1Body: `{
				"description":"v1 v2 block parity transfer",
				"metadata":{"reason":"regulatory-hold"},
				"send":{
					"asset":"USD",
					"value":"100",
					"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},
					"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}
				}
			}`,
		},
		{
			name:         "unblock v2 matches v1 unblock ledger effect",
			expectedType: cn.UNBLOCK,
			reason:       "regulatory-release",
			v2URL:        v2UnblockURL,
			v1URL:        v1UnblockURL,
			v2Body:       `{"description":"v1 v2 unblock parity transfer","asset":"USD","amount":"100","from":"@src","to":"@dst","metadata":{"reason":"regulatory-release"}}`,
			v1Body: `{
				"description":"v1 v2 unblock parity transfer",
				"metadata":{"reason":"regulatory-release"},
				"send":{
					"asset":"USD",
					"value":"100",
					"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},
					"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}
				}
			}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see file header).
			// The postgres client constructor enforces TLS by the ENV_NAME security tier and
			// refuses plaintext unless ALLOW_INSECURE_TLS=true.
			t.Setenv("ALLOW_INSECURE_TLS", "true")

			infra := setupTestInfra(t)
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			ctx := context.Background()

			// Two ledgers under the SAME org so both transactions can use IDENTICAL aliases and
			// starting balances; the only legitimate difference is then the ledger id (stripped)
			// plus per-row IDs/timestamps.
			ledgerV1 := infra.ledgerID
			ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
			seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

			v1Src, v1Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV1, "@src", "@dst", 1000)
			v2Src, v2Dst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, ledgerV2, "@src", "@dst", 1000)

			v1App := buildHumaTransactionApp(t, infra.handler, true)
			v2App := buildHumaV2DirectApp(t, infra.handler)

			// Act: economically-equivalent block/unblock on each surface. Each surface is fully
			// processed (create -> drain -> assert) BEFORE the next, because the balance-sync
			// schedule ZSET is GLOBAL, not per-ledger (see the direct parity test for the same
			// discipline).
			v1Resp := decodeTxResponse(t, postTransaction(t, v1App, tc.v1URL(infra.orgID, ledgerV1), tc.v1Body, ""), nethttp.StatusCreated)
			v1TxID := uuid.MustParse(v1Resp["id"].(string))
			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 block/unblock transaction should be APPROVED in DB")
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

			// A routeless, default-settings ledger has no Block/Unblock AccountingEntry: reaching
			// StatusCreated here (no error) is the observable proof the override resolved the
			// rubric via the Direct fallback rather than demanding a dedicated block rubric.
			v2Resp := decodeTxResponse(t, postTransaction(t, v2App, tc.v2URL(infra.orgID, ledgerV2), tc.v2Body, ""), nethttp.StatusCreated)
			v2TxID := uuid.MustParse(v2Resp["id"].(string))
			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 block/unblock transaction should be APPROVED in DB")
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

			// Balances: block/unblock move funds exactly like a direct transfer — source
			// 1000 -> 900, destination 0 -> 100, on-hold 0, on BOTH surfaces. The override
			// changes NOTHING about the accounting effect.
			requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Src), "v1 source available")
			requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Src), "v2 source available")
			requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v1Dst), "v1 dest available")
			requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, v2Dst), "v2 dest available")
			requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v1Src), "v1 source on-hold")
			requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, v2Src), "v2 source on-hold")

			// Operations: exactly 1 debit + 1 credit leg on each, EVERY leg typed
			// BLOCK/UNBLOCK, and the economic projection IDENTICAL between the two surfaces
			// (joined by alias).
			v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
			v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)
			assertBlockOpsParity(t, v1Ops, v2Ops, tc.expectedType)

			// Reason survives on both surfaces as a flat metadata key.
			assertReasonMetadata(t, v1Resp, tc.reason)
			assertReasonMetadata(t, v2Resp, tc.reason)

			// Transaction-level only: the account-level block flags stay exactly as seeded
			// (both true). A transaction-level BLOCK/UNBLOCK never flips allow_sending /
			// allow_receiving.
			for _, bID := range []uuid.UUID{v1Src, v1Dst, v2Src, v2Dst} {
				sending, receiving := fetchBalanceFlags(t, infra.pgContainer.DB, bID)
				assert.True(t, sending, "balance %s allow_sending must be untouched by a transaction-level block/unblock", bID)
				assert.True(t, receiving, "balance %s allow_receiving must be untouched by a transaction-level block/unblock", bID)
			}

			// Response deep-equal, ignoring IDs/timestamps: the v2 block/unblock response is
			// indistinguishable from the v1 equivalent for the same economic intent, INCLUDING
			// the BLOCK/UNBLOCK operation types and the reason metadata.
			assert.Equal(t, "USD", v1Resp["assetCode"])
			assert.Equal(t, "USD", v2Resp["assetCode"])

			require.Equal(t, stripVolatile(v1Resp), stripVolatile(v2Resp),
				"v2 block/unblock transaction must be indistinguishable from the v1 equivalent (ignoring IDs/timestamps)")
		})
	}
}

// =============================================================================
// 11. BLOCK / UNBLOCK VALIDATION BEFORE ANY LEDGER EFFECT: a v2 `block` / `unblock` that is
//     malformed at the decode boundary (missing required field -> canonical 400) or invalid
//     as a business rule (from == to -> ErrTransactionAmbiguous / 0090 -> 422) is rejected
//     BEFORE the funnel touches any balance, so no transaction is persisted and the seeded
//     balances are left exactly as funded.
// =============================================================================

func TestIntegration_TransactionV2BlockUnblock_ValidationBeforeLedgerEffect(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	srcID, dstID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// As in the direct-action table, the canonical code is what identifies WHICH layer
	// rejected: 0009 and 0090 are raised by two different rules and a status-only assertion
	// cannot tell a shape rejection from a business one.
	cases := []struct {
		name             string
		url              string
		body             string
		wantStatus       int
		wantCode         string
		wantBodyContains string
	}{
		{
			// Spelling a side is a rule across a pair of fields (`from`/`sources`), which no
			// struct tag expresses, so Translate owns it and rejects an unspelled side with
			// ErrMissingFieldsInRequest (0009) -> ValidationError -> 400, before the funnel.
			// Identical to the direct-action validation contract; block/unblock share the seam.
			name:             "block missing required from field",
			url:              v2BlockURL(infra.orgID, infra.ledgerID),
			body:             `{"asset":"USD","amount":"100","to":"@dst","metadata":{"reason":"regulatory-hold"}}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0009",
			wantBodyContains: "from or sources",
		},
		{
			name:             "unblock missing required from field",
			url:              v2UnblockURL(infra.orgID, infra.ledgerID),
			body:             `{"asset":"USD","amount":"100","to":"@dst","metadata":{"reason":"regulatory-release"}}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0009",
			wantBodyContains: "from or sources",
		},
		{
			// from == to is a Translate business error (ErrTransactionAmbiguous / 0090) -> 422,
			// fired before the create funnel touches any balance.
			name:       "block from equals to business error",
			url:        v2BlockURL(infra.orgID, infra.ledgerID),
			body:       `{"asset":"USD","amount":"100","from":"@src","to":"@src","metadata":{"reason":"regulatory-hold"}}`,
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   "0090",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postTransaction(t, v2App, tc.url, tc.body, "")
			body := drainBody(t, resp)

			assert.Equal(t, tc.wantStatus, resp.StatusCode, "%s should be rejected before any ledger effect; body: %s", tc.name, string(body))
			requireProblemCode(t, body, tc.wantCode)

			if tc.wantBodyContains != "" {
				assert.Contains(t, string(body), tc.wantBodyContains,
					"the rejection must name the offending field pair, which is what identifies the answering layer")
			}
		})
	}

	// No ledger effect: no transaction persisted, and both balances left exactly as seeded.
	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "no transaction should be persisted for rejected block/unblock requests")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, srcID), "source balance must be untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, dstID), "destination balance must be untouched")
}

// advancedLegV2Body spells a 100 USD transaction in the leg-array form: two explicit-amount
// debit legs and two 50% share credit legs, one per value expression the v2 leg publishes.
// Four legs is what makes it the right probe for a per-leg claim — with a single leg,
// "stamped on every leg" and "stamped once" are indistinguishable.
const advancedLegV2Body = `{"description":"v2 advanced multi-leg","asset":"USD","amount":"100",` +
	`"sources":[{"account":"@srcA","amount":"60"},{"account":"@srcB","amount":"40"}],` +
	`"destinations":[{"account":"@dstA","share":{"percentage":50}},{"account":"@dstB","share":{"percentage":50}}]}`

// advancedLegBalances are the seeded balance IDs of the four accounts advancedLegV2Body names.
type advancedLegBalances struct {
	srcA uuid.UUID
	srcB uuid.UUID
	dstA uuid.UUID
	dstB uuid.UUID
}

// seedAdvancedLegBalances seeds the four accounts advancedLegV2Body names: both sources funded
// with sourceAvailable, both destinations empty. Each source is funded independently so a leg
// that debited the wrong account shows up as a wrong per-account balance rather than being
// absorbed by a shared pool.
func seedAdvancedLegBalances(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, sourceAvailable int64) advancedLegBalances {
	t.Helper()

	seed := func(alias string, available int64) uuid.UUID {
		accountID := uuid.Must(libCommons.GenerateUUIDv7())

		params := postgrestestutil.DefaultBalanceParams()
		params.Alias = alias
		params.AssetCode = "USD"
		params.Available = decimal.NewFromInt(available)
		params.OnHold = decimal.Zero

		return postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, accountID, params)
	}

	return advancedLegBalances{
		srcA: seed("@srcA", sourceAvailable),
		srcB: seed("@srcB", sourceAvailable),
		dstA: seed("@dstA", 0),
		dstB: seed("@dstB", 0),
	}
}

// advancedLegExpectation is the persisted (Type, Amount) a single expanded leg must produce.
type advancedLegExpectation struct {
	opType string
	amount decimal.Decimal
}

// assertAdvancedLegOps asserts the persisted operation set matches, alias for alias, the
// expected type and amount of every expanded leg. The operation COUNT is pinned to the number
// of expectations, so a collapsed leg array (one operation carrying the whole total) fails
// here instead of passing a looser "at least one operation is typed correctly" check. For the
// block/unblock actions the expected type is the override on EVERY entry — that is the per-leg
// override proof.
func assertAdvancedLegOps(t *testing.T, ops []operationEconomicRow, want map[string]advancedLegExpectation) {
	t.Helper()

	require.Len(t, ops, len(want), "one operation per expanded leg")

	byAlias := indexOpsByAlias(t, ops)

	for alias, exp := range want {
		op, ok := byAlias[alias]
		require.Truef(t, ok, "no operation persisted for leg %s", alias)

		assert.Equal(t, exp.opType, op.Type, "operation[%s] type", alias)
		assert.Equal(t, "USD", op.AssetCode, "operation[%s] asset", alias)
		requireDecimalEqual(t, exp.amount, op.Amount, "operation[%s] amount", alias)
	}
}

// =============================================================================
// 12. ADVANCED (LEG-ARRAY) FORM ACROSS ALL FOUR CREATE ACTIONS: `direct`, `hold`, `block` and
//     `unblock` all accept the leg-array body and commit it, because all four share one
//     request envelope and one decode+translate seam. Each action keeps its own identity on
//     the expanded legs: hold opens the transaction as PENDING and reserves both sources,
//     block/unblock stamp their Operation.Type on EVERY one of the four resulting operations,
//     and direct settles with the plain DEBIT/CREDIT labels. The leg split is proven
//     economically — each of the four accounts moves by its own leg's value, so the amount and
//     share expressions are shown to resolve independently rather than collapsing onto one
//     leg.
// =============================================================================

func TestIntegration_TransactionV2Advanced_FourActionsAcceptLegArrays(t *testing.T) {
	cases := []struct {
		name       string
		url        func(orgID, ledgerID uuid.UUID) string
		wantStatus string
		wantOps    map[string]advancedLegExpectation
	}{
		{
			name:       "direct settles every leg with plain debit and credit labels",
			url:        v2DirectURL,
			wantStatus: cn.APPROVED,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(60)},
				"@srcB": {opType: cn.DEBIT, amount: decimal.NewFromInt(40)},
				"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(50)},
				"@dstB": {opType: cn.CREDIT, amount: decimal.NewFromInt(50)},
			},
		},
		{
			// A hold reserves the sources only; the destination credit lands on commit, so a
			// pending transaction persists one ON_HOLD operation per SOURCE leg.
			name:       "hold opens pending and reserves every source leg",
			url:        v2HoldURL,
			wantStatus: cn.PENDING,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.ONHOLD, amount: decimal.NewFromInt(60)},
				"@srcB": {opType: cn.ONHOLD, amount: decimal.NewFromInt(40)},
			},
		},
		{
			name:       "block stamps its override on every expanded leg",
			url:        v2BlockURL,
			wantStatus: cn.APPROVED,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.BLOCK, amount: decimal.NewFromInt(60)},
				"@srcB": {opType: cn.BLOCK, amount: decimal.NewFromInt(40)},
				"@dstA": {opType: cn.BLOCK, amount: decimal.NewFromInt(50)},
				"@dstB": {opType: cn.BLOCK, amount: decimal.NewFromInt(50)},
			},
		},
		{
			name:       "unblock stamps its override on every expanded leg",
			url:        v2UnblockURL,
			wantStatus: cn.APPROVED,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.UNBLOCK, amount: decimal.NewFromInt(60)},
				"@srcB": {opType: cn.UNBLOCK, amount: decimal.NewFromInt(40)},
				"@dstA": {opType: cn.UNBLOCK, amount: decimal.NewFromInt(50)},
				"@dstB": {opType: cn.UNBLOCK, amount: decimal.NewFromInt(50)},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see file header).
			// The postgres client constructor enforces TLS by the ENV_NAME security tier and
			// refuses plaintext unless ALLOW_INSECURE_TLS=true.
			t.Setenv("ALLOW_INSECURE_TLS", "true")

			infra := setupTestInfra(t)
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			ctx := context.Background()

			balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

			v2App := buildHumaV2DirectApp(t, infra.handler)

			resp := decodeTxResponse(t, postTransaction(t, v2App, tc.url(infra.orgID, infra.ledgerID), advancedLegV2Body, ""), nethttp.StatusCreated)
			txID := uuid.MustParse(resp["id"].(string))

			assert.Equal(t, tc.wantStatus, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
				"an advanced-body %s must open the transaction as %s", tc.name, tc.wantStatus)
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

			assertAdvancedLegOps(t, fetchOperationRows(t, infra.pgContainer.DB, txID), tc.wantOps)

			// Economic proof that the legs resolved independently: each source moved by its own
			// leg's value (explicit 60 and explicit 40). A pending hold reserves the sources
			// instead of debiting them, and leaves both destinations untouched.
			if tc.wantStatus == cn.PENDING {
				requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available after hold")
				requireDecimalEqual(t, decimal.NewFromInt(60), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcA), "@srcA on-hold after hold")
				requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB available after hold")
				requireDecimalEqual(t, decimal.NewFromInt(40), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcB), "@srcB on-hold after hold")
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA untouched before commit")
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstB), "@dstB untouched before commit")

				return
			}

			requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available (explicit 60 leg)")
			requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB available (explicit 40 leg)")
			requireDecimalEqual(t, decimal.NewFromInt(50), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA available (50% share leg)")
			requireDecimalEqual(t, decimal.NewFromInt(50), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstB), "@dstB available (50% share leg)")

			for _, bID := range []uuid.UUID{balances.srcA, balances.srcB, balances.dstA, balances.dstB} {
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, bID), "balance %s on-hold must be zero on a settled transaction", bID)
			}
		})
	}
}

// remainingLegV1Body spells a 100 USD transaction in the v1 detailed form: one source leg
// taking an explicit 60, a second taking the remainder, and a single destination taking the
// full 100. `remaining` is a v1/DSL expression only — the v2 surface publishes no such field,
// so a v2 body cannot spell this shape.
const remainingLegV1Body = `{
	"description":"remaining leg",
	"send":{
		"asset":"USD","value":"100",
		"source":{"from":[
			{"accountAlias":"@srcA","amount":{"asset":"USD","value":"60"}},
			{"accountAlias":"@srcB","remaining":"remaining"}
		]},
		"distribute":{"to":[{"accountAlias":"@dstA","amount":{"asset":"USD","value":"100"}}]}
	}
}`

// sumOperationAmountsByType totals the persisted operation amounts per operation type, so a
// transaction can be checked for whether its debits and credits actually balance.
func sumOperationAmountsByType(ops []operationEconomicRow) map[string]decimal.Decimal {
	totals := make(map[string]decimal.Decimal, len(ops))

	for _, op := range ops {
		totals[op.Type] = totals[op.Type].Add(op.Amount)
	}

	return totals
}

// =============================================================================
// 13. KNOWN DEFECT — `remaining` LEG DROPPED ON THE v1 SURFACE: a leg whose value is the
//     `remaining` expression resolves correctly during validation (so the balance check
//     passes) but contributes NO balance movement and NO operation row, and the transaction is
//     nevertheless committed as APPROVED with debits and credits that do not sum to each other.
//
//     This test asserts the CURRENT WRONG behavior on purpose, and it is v1-ONLY. The defect
//     sits in the shared create funnel, so fixing it changes released v1 behavior and needs its
//     own release; the v2 surface answers it by publishing no `remaining` expression at all, so
//     there is no v2 spelling of this shape to pin (see
//     TestV2LegInput_NoRemainingExpression). The pin lives in this file because it records what
//     the v2 advanced form deliberately does NOT inherit. When the funnel is fixed, this test
//     goes red and is the place to record the corrected v1 contract.
// =============================================================================

func TestIntegration_TransactionV1Detailed_RemainingLegDropped_KnownDefect(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	app := buildHumaTransactionApp(t, infra.handler, true)

	resp := decodeTxResponse(t, postTransaction(t, app, v1JSONURL(infra.orgID, infra.ledgerID), remainingLegV1Body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(resp["id"].(string))

	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
		"the transaction is committed despite the dropped leg")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	ops := fetchOperationRows(t, infra.pgContainer.DB, txID)

	// The remaining leg produces no operation row at all: two of the three legs persist.
	require.Len(t, ops, 2, "the remaining leg contributes no operation row")

	byAlias := indexOpsByAlias(t, ops)
	_, remainingLegPersisted := byAlias["@srcB"]
	assert.False(t, remainingLegPersisted, "@srcB is the remaining leg and persists no operation")

	// And no balance movement: @srcB keeps every unit it was seeded with.
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB),
		"@srcB is untouched even though it was resolved to 40 during validation")

	// The committed result is unbalanced: 60 debited against 100 credited.
	totals := sumOperationAmountsByType(ops)
	requireDecimalEqual(t, decimal.NewFromInt(60), totals[cn.DEBIT], "persisted debit total")
	requireDecimalEqual(t, decimal.NewFromInt(100), totals[cn.CREDIT], "persisted credit total")
	assert.False(t, totals[cn.DEBIT].Equal(totals[cn.CREDIT]),
		"the committed transaction does not balance — this is the defect being pinned")
}

// =============================================================================
// 14. FUNDING VIA A LEG NAMING THE EXTERNAL ACCOUNT: `@external/<ASSET>` reaches the ledger
//     through a `sources` leg and settles. The v2 surface publishes no inflow/outflow action,
//     so naming that alias explicitly is the ONLY way to spell a deposit — and the alias
//     contains `/`, which the registered account-alias charset excludes. This test is the
//     end-to-end lock on that: the leg guard rejects `#` alone, so a charset-based guard
//     replacing it would fail here rather than silently 400 every deposit in production.
// =============================================================================

// seedExternalFundingBalances seeds the pair a deposit needs: the ledger's external account
// (overdraft-capable, so it can fund from zero) and a plain destination account.
func seedExternalFundingBalances(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID) (external, destination uuid.UUID) {
	t.Helper()

	externalParams := postgrestestutil.DefaultBalanceParams()
	externalParams.Alias = externalUSDAlias
	externalParams.AssetCode = "USD"
	externalParams.Available = decimal.Zero
	externalParams.OnHold = decimal.Zero
	externalParams.AccountType = "external"

	external = postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID,
		uuid.Must(libCommons.GenerateUUIDv7()), externalParams)

	destinationParams := postgrestestutil.DefaultBalanceParams()
	destinationParams.Alias = "@alice"
	destinationParams.AssetCode = "USD"
	destinationParams.Available = decimal.Zero
	destinationParams.OnHold = decimal.Zero

	destination = postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID,
		uuid.Must(libCommons.GenerateUUIDv7()), destinationParams)

	return external, destination
}

// externalUSDAlias is the alias every ledger's USD external account carries. Spelled from the
// production constant so a change to the prefix surfaces here.
var externalUSDAlias = cn.DefaultExternalAccountAliasPrefix + "USD"

func TestIntegration_TransactionV2Advanced_ExternalAccountLegFundsAccount(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	externalID, aliceID := seedExternalFundingBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	body := `{"description":"fund alice","asset":"USD","amount":"100",` +
		`"sources":[{"account":"` + externalUSDAlias + `","amount":"100"}],` +
		`"destinations":[{"account":"@alice","amount":"100"}]}`

	resp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(resp["id"].(string))

	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
		"a deposit spelled with an external-account leg must settle")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Both legs persisted, each against its own alias: the `/` in the external alias survived
	// the leg guard and the alias round-tripped through the funnel's per-leg map.
	byAlias := indexOpsByAlias(t, fetchOperationRows(t, infra.pgContainer.DB, txID))

	externalOp, ok := byAlias[externalUSDAlias]
	require.Truef(t, ok, "the external-account leg must persist an operation under %s", externalUSDAlias)
	assert.Equal(t, cn.DEBIT, externalOp.Type, "the external leg is the debit side of a deposit")
	requireDecimalEqual(t, decimal.NewFromInt(100), externalOp.Amount, "external leg amount")

	aliceOp, ok := byAlias["@alice"]
	require.True(t, ok, "the destination leg must persist an operation")
	assert.Equal(t, cn.CREDIT, aliceOp.Type)
	requireDecimalEqual(t, decimal.NewFromInt(100), aliceOp.Amount, "destination leg amount")

	// Economic proof: the funds landed, drawn from the external account's overdraft.
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, aliceID),
		"@alice must hold the deposited funds")
	requireDecimalEqual(t, decimal.NewFromInt(-100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, externalID),
		"the external account funds the deposit from its overdraft")
}

// =============================================================================
// 15. UNBALANCED LEG ARRAY: `amount` is mandatory in the array form BECAUSE it is the total
//     the legs divide, and Translate deliberately does not check that the legs sum to it —
//     the create funnel owns that rule (ErrTransactionValueMismatch / 0073 -> 422). Every
//     other leg-array test in this file uses legs that sum exactly, so a translation bug that
//     dropped a leg or mis-mapped the total would leave them all green while committing a
//     transaction whose debits and credits do not match. This is the single most important
//     invariant of the form, and it is asserted for both per-leg value expressions.
// =============================================================================

func TestIntegration_TransactionV2Advanced_UnbalancedLegsRejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			// Explicit amounts: 60 + 30 = 90 against a declared total of 100.
			name: "explicit amount legs that do not sum to the declared total",
			body: `{"description":"unbalanced explicit legs","asset":"USD","amount":"100",` +
				`"sources":[{"account":"@srcA","amount":"60"},{"account":"@srcB","amount":"30"}],` +
				`"destinations":[{"account":"@dstA","amount":"100"}]}`,
		},
		{
			// Shares: 60% + 30% = 90% of the total, so the resolved legs sum to 90.
			name: "share legs whose percentages do not sum to the whole total",
			body: `{"description":"unbalanced share legs","asset":"USD","amount":"100",` +
				`"sources":[{"account":"@srcA","share":{"percentage":60}},{"account":"@srcB","share":{"percentage":30}}],` +
				`"destinations":[{"account":"@dstA","amount":"100"}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see file header).
			t.Setenv("ALLOW_INSECURE_TLS", "true")

			infra := setupTestInfra(t)
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

			v2App := buildHumaV2DirectApp(t, infra.handler)

			resp := postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), tc.body, "")
			body := drainBody(t, resp)

			assert.Equal(t, nethttp.StatusUnprocessableEntity, resp.StatusCode,
				"legs that do not sum to the declared total are a funnel business error -> 422; body: %s", string(body))
			requireProblemCode(t, body, "0073")

			// No ledger effect: the rejection lands before anything is persisted or moved.
			assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
				"an unbalanced leg array must not persist a transaction")

			for _, bID := range []uuid.UUID{balances.srcA, balances.srcB} {
				requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, bID),
					"source balance %s must be untouched", bID)
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, bID),
					"source balance %s on-hold must be untouched", bID)
			}

			for _, bID := range []uuid.UUID{balances.dstA, balances.dstB} {
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, bID),
					"destination balance %s must be untouched", bID)
			}
		})
	}
}

// =============================================================================
// 16. percentageOfPercentage NARROWS THE SHARE: the resolver computes
//     total x (percentage/100) x (percentageOfPercentage/100), treating a zero
//     percentageOfPercentage as 100. So on a 100 total, `{"percentage":60,
//     "percentageOfPercentage":50}` must move exactly 30 and `{"percentage":70}` exactly 70.
//     Every other assertion on this field checks only that the submitted int64 was copied onto
//     the canonical struct, which stays true if the two factors were swapped or one was wired
//     to the wrong field. This test measures the MONEY the pair resolves to: the persisted
//     operation amount and the balance delta.
// =============================================================================

func TestIntegration_TransactionV2Advanced_PercentageOfPercentageResolvesNarrowedAmount(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	// 60% narrowed to 50% of itself is 30 of the 100 total; the sibling leg takes the
	// remaining 70 as a plain share, so the two resolved legs sum to the declared total and
	// the funnel's balance rule cannot be what makes the request pass or fail.
	body := `{"description":"narrowed share","asset":"USD","amount":"100",` +
		`"sources":[{"account":"@srcA","amount":"100"}],` +
		`"destinations":[{"account":"@dstA","share":{"percentage":60,"percentageOfPercentage":50}},` +
		`{"account":"@dstB","share":{"percentage":70}}]}`

	resp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(resp["id"].(string))

	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
		"a doubly-narrowed share that still sums to the total must settle")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// The persisted operation amounts ARE the resolved values.
	assertAdvancedLegOps(t, fetchOperationRows(t, infra.pgContainer.DB, txID), map[string]advancedLegExpectation{
		"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(100)},
		"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(30)},
		"@dstB": {opType: cn.CREDIT, amount: decimal.NewFromInt(70)},
	})

	// And the balance deltas agree with them: 30 is 50% of 60% of 100, not 60, not 50, and
	// not 30 arrived at by swapping the two factors onto each other's fields (which would
	// also yield 30 only because 60x50 is symmetric — hence the asymmetric sibling leg above
	// and the 70 assertion, which no swap reproduces).
	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available after the debit")
	requireDecimalEqual(t, decimal.NewFromInt(30), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA received 50% of its 60% share")
	requireDecimalEqual(t, decimal.NewFromInt(70), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstB), "@dstB received its full 70% share")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB is named by no leg and must be untouched")
}

// operationsByAliasAndType keys an operation set by "<alias>/<type>". Unlike indexOpsByAlias
// it tolerates one alias appearing more than once in a transaction, which is exactly the shape
// the same-account-on-both-sides pin below records.
func operationsByAliasAndType(t *testing.T, ops []operationEconomicRow) map[string]operationEconomicRow {
	t.Helper()

	out := make(map[string]operationEconomicRow, len(ops))

	for _, op := range ops {
		key := op.AccountAlias + "/" + op.Type

		_, dup := out[key]
		require.Falsef(t, dup, "two operations share alias %s and type %s", op.AccountAlias, op.Type)

		out[key] = op
	}

	return out
}

// =============================================================================
// 17. SAME ACCOUNT ON BOTH SIDES OF ONE TRANSACTION. Per-side exclusivity newly legalises a
//     scalar source paired with a destination ARRAY, a shape that could not be spelled before,
//     and the scalar From == To check cannot see it (To is empty). The funnel's ambiguity guard
//     is the only thing left, and it compares the two sides INDEX-POSITIONALLY — so it fires
//     when the alias sits at the same index on both sides and not otherwise.
//
//     Both outcomes are pinned. The caught row records an accidental guarantee that no
//     refactor should be free to drop silently. The escaping row is a KNOWN-DEFECT pin in the
//     same discipline as the v1 `remaining` pin above: it asserts the CURRENT WRONG behavior on
//     purpose, so tightening the funnel guard goes red here and this is the place to record the
//     corrected contract. The escaping shape was already reachable through array/array bodies
//     before per-side exclusivity existed, so this pins parity with the released surface rather
//     than a regression introduced by it.
// =============================================================================

func TestIntegration_TransactionV2_SameAccountOnBothSides(t *testing.T) {
	t.Run("same alias at the same index on both sides is rejected", func(t *testing.T) {
		// NOT parallel: process-global huma state (see file header).
		t.Setenv("ALLOW_INSECURE_TLS", "true")

		infra := setupTestInfra(t)
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		selfID, otherID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@self", "@other", 1000)

		v2App := buildHumaV2DirectApp(t, infra.handler)

		body := `{"description":"self transfer","asset":"USD","amount":"100","from":"@self",` +
			`"destinations":[{"account":"@self","amount":"100"}]}`

		resp := postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), body, "")
		respBody := drainBody(t, resp)

		assert.Equal(t, nethttp.StatusUnprocessableEntity, resp.StatusCode,
			"one alias on both sides at index 0 is an ambiguous transaction -> 422; body: %s", string(respBody))
		requireProblemCode(t, respBody, "0090")

		assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
			"an ambiguous transaction must not persist")
		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, selfID), "@self must be untouched")
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, otherID), "@other must be untouched")
	})

	t.Run("known defect same alias at different indexes is accepted and both debited and credited", func(t *testing.T) {
		// NOT parallel: process-global huma state (see file header).
		t.Setenv("ALLOW_INSECURE_TLS", "true")

		infra := setupTestInfra(t)
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		ctx := context.Background()

		selfID, otherID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@self", "@other", 1000)

		v2App := buildHumaV2DirectApp(t, infra.handler)

		// @self is the whole source side and ALSO the second destination leg. The guard misses
		// it because it does not sit at index 0 of the destination array.
		body := `{"description":"self at index one","asset":"USD","amount":"100","from":"@self",` +
			`"destinations":[{"account":"@other","amount":"60"},{"account":"@self","amount":"40"}]}`

		resp := decodeTxResponse(t, postTransaction(t, v2App, v2DirectURL(infra.orgID, infra.ledgerID), body, ""), nethttp.StatusCreated)
		txID := uuid.MustParse(resp["id"].(string))

		assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
			"the transaction is committed despite naming one account on both sides")
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		// @self carries BOTH a debit and a credit inside one transaction — the defect.
		byKey := operationsByAliasAndType(t, fetchOperationRows(t, infra.pgContainer.DB, txID))

		selfDebit, ok := byKey["@self/"+cn.DEBIT]
		require.True(t, ok, "@self is the source side and must carry a debit")
		requireDecimalEqual(t, decimal.NewFromInt(100), selfDebit.Amount, "@self debit is the whole total")

		selfCredit, ok := byKey["@self/"+cn.CREDIT]
		require.True(t, ok, "@self is also a destination leg and carries a credit in the SAME transaction")
		requireDecimalEqual(t, decimal.NewFromInt(40), selfCredit.Amount, "@self credit is its destination leg's value")

		otherCredit, ok := byKey["@other/"+cn.CREDIT]
		require.True(t, ok, "@other must carry its own credit")
		requireDecimalEqual(t, decimal.NewFromInt(60), otherCredit.Amount, "@other credit")

		// Net effect: @self loses 100 and regains 40.
		requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, selfID),
			"@self is debited 100 and credited 40 in one transaction — this is the behavior being pinned")
		requireDecimalEqual(t, decimal.NewFromInt(60), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, otherID), "@other available")
	})
}

// =============================================================================
// 18. MULTI-LEG HOLD -> COMMIT: the four-action advanced test proves a multi-leg hold RESERVES
//     both sources and then returns, so nothing yet shows that a four-leg pending transaction
//     SETTLES. Commit is where the two ON_HOLD reservations must release into debits and the two
//     share-resolved credits must land; a per-leg bug in the settle path (a released reservation
//     applied to the wrong leg, a share re-resolved against a stale total) is invisible until
//     the final balances are read. This test commits the multi-leg hold and asserts all four
//     final balances with on-hold back to zero.
// =============================================================================

func TestIntegration_TransactionV2Advanced_MultiLegHoldCommitSettles(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	holdResp := decodeTxResponse(t, postTransaction(t, v2App, v2HoldURL(infra.orgID, infra.ledgerID), advancedLegV2Body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(holdResp["id"].(string))

	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
		"a multi-leg hold opens the transaction as PENDING")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// After the hold: each source reserved by ITS OWN leg's value, both destinations untouched.
	requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available after hold")
	requireDecimalEqual(t, decimal.NewFromInt(60), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcA), "@srcA on-hold after hold")
	requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB available after hold")
	requireDecimalEqual(t, decimal.NewFromInt(40), postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcB), "@srcB on-hold after hold")

	// Commit through the v1 lifecycle endpoint; the v2-held transaction shares the handler,
	// use cases and DB, so it settles exactly like a v1 pending transaction.
	_ = decodeTxResponse(t, postTransaction(t, infra.app, v1CommitURL(infra.orgID, infra.ledgerID, txID), "", ""), nethttp.StatusCreated)

	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
		"a committed multi-leg hold flips to APPROVED")
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// After the commit: BOTH reservations released (on-hold back to zero, available unchanged
	// from the reserved figure) and BOTH share-resolved credits landed.
	requireDecimalEqual(t, decimal.NewFromInt(940), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcA), "@srcA available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcA), "@srcA on-hold released")
	requireDecimalEqual(t, decimal.NewFromInt(960), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.srcB), "@srcB available after commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.srcB), "@srcB on-hold released")
	requireDecimalEqual(t, decimal.NewFromInt(50), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstA), "@dstA credited its 50% share on commit")
	requireDecimalEqual(t, decimal.NewFromInt(50), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, balances.dstB), "@dstB credited its 50% share on commit")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.dstA), "@dstA on-hold stays zero")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, balances.dstB), "@dstB on-hold stays zero")

	// Operation TYPE set across the full lifecycle: one ON_HOLD per source leg (the
	// reservation), one DEBIT per source leg (the release) and one CREDIT per destination leg
	// (the apply). Asserting the multiset — not merely the count — catches a settle path that
	// emitted six operations with the wrong type mix.
	commitOps := fetchOperationRows(t, infra.pgContainer.DB, txID)

	commitOpTypes := make([]string, 0, len(commitOps))
	for _, op := range commitOps {
		commitOpTypes = append(commitOpTypes, op.Type)
	}

	assert.ElementsMatch(t,
		[]string{cn.ONHOLD, cn.ONHOLD, cn.DEBIT, cn.DEBIT, cn.CREDIT, cn.CREDIT},
		commitOpTypes,
		"a committed two-source two-destination hold carries two of each lifecycle leg")
}
