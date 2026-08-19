// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"slices"
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
// `direct` transaction endpoint (POST /v2/transactions/direct). It mounts BOTH the
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

// v2CreateURL builds the v2 create path for an action. It names no organization and no
// ledger: a v2 create is scoped by the organization and ledger its request body names.
func v2CreateURL(action string) string {
	return "/v2/transactions/" + action
}

// v2ScopedBody restates a v2 body's placeholder scope as the organization and ledger the caller
// seeded. The shared bodies are package-level constants, so they spell one fixed placeholder
// pair, while the seeded pair is generated per test — and it is the body that decides which
// ledger a create posts against.
func v2ScopedBody(body string, orgID, ledgerID uuid.UUID) string {
	return strings.NewReplacer(
		v2ScopeOrgID, orgID.String(),
		v2ScopeLedgerID, ledgerID.String(),
	).Replace(body)
}

// postV2Create posts a v2 create body to the scope-free action path under the given scope,
// restating that scope inside the body.
func postV2Create(t *testing.T, app *fiber.App, action string, orgID, ledgerID uuid.UUID, body, idempotencyKey string) *nethttp.Response {
	t.Helper()

	return postTransaction(t, app, v2CreateURL(action), v2ScopedBody(body, orgID, ledgerID), idempotencyKey)
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

// fetchOperationRows returns the economic projection of every operation for a transaction.
// The row order it returns is not a total order over the set; put the result through
// sortOperationRows before any index-for-index comparison.
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

	require.Truef(t, want.Equal(got), "expected decimal %s, got %s (%s)", want.String(), got.String(), decimalContext(msgAndArgs))
}

// decimalContext renders a caller's trailing message. A leading string followed by arguments is
// treated as a format and substituted, so the values land inline rather than in a bracketed tail.
// A leading string on its own is returned verbatim: formatting it against an empty argument list
// would turn any literal percent sign into a bogus verb and corrupt the very message meant to
// explain the failure.
func decimalContext(msgAndArgs []any) string {
	if len(msgAndArgs) == 0 {
		return "no context"
	}

	format, ok := msgAndArgs[0].(string)
	if !ok {
		return fmt.Sprintf("%v", msgAndArgs)
	}

	if len(msgAndArgs) == 1 {
		return format
	}

	return fmt.Sprintf(format, msgAndArgs[1:]...)
}

// volatileResponseKeys are the fields deleted (recursively) before a v1↔v2 response
// deep-equal, so two economically-identical transactions in two ledgers compare equal on
// everything that carries economic meaning. Two kinds of keys live here: identity/timestamp
// fields that legitimately differ per row, and the deprecated fields the /v2 wire contract
// deliberately drops while v1 keeps emitting them (transaction-level `chartOfAccountsGroupName`
// and `route`, operation-level `chartOfAccounts` and `route` — see transaction_v2_output.go).
// The drop itself is pinned by the field-removal and mirror-reads suites, which read the raw
// maps on purpose; here the deprecated keys are just noise outside the economic envelope.
var volatileResponseKeys = map[string]struct{}{
	"id":                       {},
	"transactionId":            {},
	"parentTransactionId":      {},
	"ledgerId":                 {},
	"organizationId":           {},
	"accountId":                {},
	"balanceId":                {},
	"createdAt":                {},
	"updatedAt":                {},
	"deletedAt":                {},
	"route":                    {},
	"routeId":                  {},
	"chartOfAccounts":          {},
	"chartOfAccountsGroupName": {},
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
// renameV2LegKeys renames a /v2 response's `debit`/`credit` keys back to the v1 `source`/
// `destination` spelling, IN PLACE. The rename is the ONE deliberate difference the /v2 wire
// contract introduces on the Transaction body — every other field name and value stays
// identical — so a v1<->v2 parity deep-equal normalizes it here rather than treating it as a
// spurious mismatch.
func renameV2LegKeys(resp map[string]any) map[string]any {
	if debit, ok := resp["debit"]; ok {
		resp["source"] = debit
		delete(resp, "debit")
	}

	if credit, ok := resp["credit"]; ok {
		resp["destination"] = credit
		delete(resp, "credit")
	}

	return resp
}

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
	equivalentV2Body = `{"description":"v1 v2 parity transfer","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

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

	v2Resp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, ledgerV2, equivalentV2Body, ""), nethttp.StatusCreated)
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

	require.Equal(t, stripVolatile(v1Resp), stripVolatile(renameV2LegKeys(v2Resp)),
		"v2 direct transaction must be indistinguishable from the v1 /json equivalent (ignoring IDs/timestamps)")
}

// sortOperationRows returns the projection in a total order over EVERY field
// assertOperationSetsEqual compares, so two sets read from two ledgers line up index-for-index.
// A multi-leg transaction persists several rows sharing one Type, and the sort is not stable, so
// a key that left any compared field out of the ordering would make that field's comparison
// arbitrary between two rows tying on the rest.
func sortOperationRows(rows []operationEconomicRow) []operationEconomicRow {
	sorted := slices.Clone(rows)

	slices.SortFunc(sorted, func(x, y operationEconomicRow) int {
		if c := strings.Compare(x.Type, y.Type); c != 0 {
			return c
		}

		if c := strings.Compare(x.AccountAlias, y.AccountAlias); c != 0 {
			return c
		}

		if c := strings.Compare(x.AssetCode, y.AssetCode); c != 0 {
			return c
		}

		if c := x.Amount.Cmp(y.Amount); c != 0 {
			return c
		}

		if c := x.AvailableAfter.Cmp(y.AvailableAfter); c != 0 {
			return c
		}

		return x.OnHoldAfter.Cmp(y.OnHoldAfter)
	})

	return sorted
}

// assertOperationSetsEqual asserts two operation sets carry identical economic content
// leg-for-leg. Leg order in a persisted set is not contract, so both sides are put through
// sortOperationRows before the index-for-index comparison.
func assertOperationSetsEqual(t *testing.T, a, b []operationEconomicRow) {
	t.Helper()

	require.Equal(t, len(a), len(b), "operation set sizes differ")

	a, b = sortOperationRows(a), sortOperationRows(b)

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
	url := v2CreateURL("direct")

	// The table deliberately spans FOUR rejection layers, and the canonical code plus the
	// layer-specific text in the body is the only thing that identifies which one answered.
	// Asserting status alone would let a row pass on a 400 raised anywhere — including a
	// layer that has no business seeing the body at all.
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
		// wantBodyContains distinguishes the layers that all answer with 0009. The Translate
		// side rule names the field PAIR in its detail. The struct-tag layer phrases its
		// rejection as "<field> is a required field" in the errors array, and that phrasing
		// is unique to it — the indexed reference alone is not, because Translate's own
		// leg-account check emits the same reference.
		wantBodyContains string
	}{
		{
			// Both sides are required, non-empty leg arrays (`debits`/`credits`); no scalar
			// spelling exists anymore. An absent debits key decodes to a nil slice, and the
			// `min=1` struct tag (not `required`) catches it at the decode boundary: the
			// validator-error translator routes a `min`-tag failure to the generic
			// ErrBadRequest (0047) bucket, distinct from the `required`-tag bucket
			// (ErrMissingFieldsInRequest / 0009) the next row pins. Translate's own
			// presence check (validateSidesPresent, ALSO 0009) never runs here — it exists
			// for a caller that builds the input in Go and skips the decoder entirely, which
			// an HTTP request never does.
			name:             "missing debits field",
			body:             `{"asset":"USD","amount":"100","credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0047",
			wantBodyContains: "debits",
		},
		{
			// The struct-tag `required` layer. It answers with the SAME 0009 the Translate
			// side rule uses, so only the per-field entry in the errors array tells the two
			// apart — which is why this row pins that text and the row above pins the field.
			name:             "missing required asset field",
			body:             `{"amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
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
			// A leg naming no alias. The obligation is enforced TWICE — as a `required`
			// struct tag and imperatively in Translate — and both renderings mention the
			// indexed reference, so naming the leg is NOT what tells the two apart. The tag
			// layer runs first (DecodeAndValidate validates the struct before Translate is
			// reached) and only IT phrases the rejection as "<ref> is a required field", so
			// that full phrasing is what pins the answering layer. The offending leg is the
			// SECOND one, so a rendering that hardcoded index 0 fails this row too.
			name:             "leg without an alias",
			body:             `{"asset":"USD","amount":"100","debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"100"},{"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0009",
			wantBodyContains: "debits[1].alias is a required field",
		},
		{
			// A leg filling NEITHER value expression: 0072, naming the offending side AND
			// the offending leg's index. The leg here is the SECOND one, so a message that
			// hardcoded index 0 fails this row.
			name:             "leg with no value expression",
			body:             `{"asset":"USD","amount":"100","debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"100"},{"alias":"@srcB",` + v2ScopeJSON + `}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0072",
			wantBodyContains: "'debits[1]'",
		},
		{
			// A leg whose explicit amount is zero. Unlike the four rows above this is a
			// VALUE rule, not a shape rule, so it is a 422 rather than a 400 — the status
			// and the code move together and both are pinned.
			name:       "leg with a zero amount",
			body:       `{"asset":"USD","amount":"100","debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"0"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`,
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   "0125",
		},
	}
	// A row asserting the retired scalar-vs-array mutual-exclusivity rule (0498, "a side
	// spelled both scalar and as a leg array") used to live here. It tested a shape
	// (`from`/`to` scalar fields alongside `sources`/`destinations` arrays) that
	// CreateTransactionV2Input no longer has a Go field for — the request carries ONLY
	// `debits`/`credits` arrays now — so the rule it exercised cannot be expressed anymore
	// and the row was removed rather than reworded onto an unrelated case.

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postTransaction(t, v2App, url, v2ScopedBody(tc.body, infra.orgID, infra.ledgerID), "")
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
// 3. BUSINESS RULE: naming one account on both sides is a business error -> 422, with NO
//    ledger effect. The rule belongs to the funnel, not to Translate, and the funnel catches
//    it on its SECOND validate — after the idempotency claim, the ledger-settings read and
//    the fee engine. No balance moves, but the rejection is not an early one.
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

	resp := postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, `{"asset":"USD","amount":"100","debits":[{"alias":"@same",`+v2ScopeJSON+`,"amount":"100"}],"credits":[{"alias":"@same",`+v2ScopeJSON+`,"amount":"100"}]}`, "")
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
	url := v2CreateURL("direct")

	first := postTransaction(t, v2App, url, v2ScopedBody(equivalentV2Body, infra.orgID, infra.ledgerID), idempotencyKey)
	firstResult := decodeTxResponse(t, first, nethttp.StatusCreated)
	assert.Equal(t, "false", first.Header.Get("X-Idempotency-Replayed"), "first call must not be a replay")

	firstTxID := firstResult["id"].(string)

	// Wait for the async idempotency-value store before replaying (see helper doc).
	waitForIdempotencyStored(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, idempotencyKey)

	second := postTransaction(t, v2App, url, v2ScopedBody(equivalentV2Body, infra.orgID, infra.ledgerID), idempotencyKey)
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

	v2CrossBody := `{"description":"cross dedup","asset":"USD","amount":"100","debits":[{"alias":"@xsrc",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@xdst",` + v2ScopeJSON + `,"amount":"100"}]}`
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

	v2Cross := postTransaction(t, v2App, url, v2ScopedBody(v2CrossBody, infra.orgID, infra.ledgerID), "")
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

	resp := postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID,
		`{"description":"v2 insufficient funds","asset":"USD","amount":"5000","debits":[{"alias":"@src",`+v2ScopeJSON+`,"amount":"5000"}],"credits":[{"alias":"@dst",`+v2ScopeJSON+`,"amount":"5000"}]}`, "")
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
	holdParityV2Body = `{"description":"v1 v2 hold parity transfer","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

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

	v2Resp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, ledgerV2, holdParityV2Body, ""), nethttp.StatusCreated)
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

	require.Equal(t, stripVolatile(v1Resp), stripVolatile(renameV2LegKeys(v2Resp)),
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
	holdResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, holdParityV2Body, ""), nethttp.StatusCreated)
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
	url := v2CreateURL("hold")

	first := postTransaction(t, v2App, url, v2ScopedBody(holdParityV2Body, infra.orgID, infra.ledgerID), idempotencyKey)
	firstResult := decodeTxResponse(t, first, nethttp.StatusCreated)
	assert.Equal(t, "false", first.Header.Get("X-Idempotency-Replayed"), "first hold call must not be a replay")

	firstTxID := firstResult["id"].(string)

	// Wait for the async idempotency-value store before replaying (see helper doc).
	waitForIdempotencyStored(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, idempotencyKey)

	second := postTransaction(t, v2App, url, v2ScopedBody(holdParityV2Body, infra.orgID, infra.ledgerID), idempotencyKey)
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
	body := `{"description":"direct hold cross dedup","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

	directResp := postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, body, "")
	directResult := decodeTxResponse(t, directResp, nethttp.StatusCreated)
	assert.Equal(t, "false", directResp.Header.Get("X-Idempotency-Replayed"), "direct create must not be a replay")

	directTxID := uuid.MustParse(directResult["id"].(string))

	holdResp := postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, body, "")
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

// v1BlockURL / v1UnblockURL build the concrete v1 block/unblock paths. The v1 block/unblock
// Huma ops are registered by RegisterTransactionRoutes (inside buildHumaTransactionApp) and
// enter the SAME createTransactionShell funnel the v2 actions do, which is what makes them the
// parity reference for the v2 block/unblock actions. Only where each surface reads the scope
// differs: v1 from these path segments, v2 from the request body.
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
		v2Action     string
		v1URL        func(orgID, ledgerID uuid.UUID) string
		v2Body       string
		v1Body       string
	}{
		{
			name:         "block v2 matches v1 block ledger effect",
			expectedType: cn.BLOCK,
			reason:       "regulatory-hold",
			v2Action:     "block",
			v1URL:        v1BlockURL,
			v2Body:       `{"description":"v1 v2 block parity transfer","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}],"metadata":{"reason":"regulatory-hold"}}`,
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
			v2Action:     "unblock",
			v1URL:        v1UnblockURL,
			v2Body:       `{"description":"v1 v2 unblock parity transfer","asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}],"metadata":{"reason":"regulatory-release"}}`,
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
			v2Resp := decodeTxResponse(t, postV2Create(t, v2App, tc.v2Action, infra.orgID, ledgerV2, tc.v2Body, ""), nethttp.StatusCreated)
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

			require.Equal(t, stripVolatile(v1Resp), stripVolatile(renameV2LegKeys(v2Resp)),
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
			// A missing `debits` array is caught at the decode boundary: an omitted key
			// decodes to a nil slice, and the `min=1` struct tag (not `required`) routes
			// through the generic ErrBadRequest (0047) bucket — identical to the direct-action
			// validation contract's "missing debits field" row; block/unblock share the seam.
			name:             "block missing required debits field",
			url:              v2CreateURL("block"),
			body:             `{"asset":"USD","amount":"100","credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}],"metadata":{"reason":"regulatory-hold"}}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0047",
			wantBodyContains: "debits",
		},
		{
			name:             "unblock missing required debits field",
			url:              v2CreateURL("unblock"),
			body:             `{"asset":"USD","amount":"100","credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}],"metadata":{"reason":"regulatory-release"}}`,
			wantStatus:       nethttp.StatusBadRequest,
			wantCode:         "0047",
			wantBodyContains: "debits",
		},
		{
			// One account on both sides is ErrTransactionAmbiguous (0090) -> 422, answered by the
			// funnel's second validate. No balance moves.
			name:       "block from equals to business error",
			url:        v2CreateURL("block"),
			body:       `{"asset":"USD","amount":"100","debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"credits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],"metadata":{"reason":"regulatory-hold"}}`,
			wantStatus: nethttp.StatusUnprocessableEntity,
			wantCode:   "0090",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postTransaction(t, v2App, tc.url, v2ScopedBody(tc.body, infra.orgID, infra.ledgerID), "")
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
	`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"40"}],` +
	`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":50}},{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":50}}]}`

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

// directAdvancedLegOps is the settled operation set advancedLegV2Body produces on the DIRECT
// action: each source moved by its own explicit-amount leg, each destination by its own share
// leg. Shared by every subject that commits that body on that action, so the expectation exists
// once and cannot drift between them.
func directAdvancedLegOps() map[string]advancedLegExpectation {
	return map[string]advancedLegExpectation{
		"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(60)},
		"@srcB": {opType: cn.DEBIT, amount: decimal.NewFromInt(40)},
		"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(50)},
		"@dstB": {opType: cn.CREDIT, amount: decimal.NewFromInt(50)},
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
		action     string
		wantStatus string
		wantOps    map[string]advancedLegExpectation
	}{
		{
			name:       "direct settles every leg with plain debit and credit labels",
			action:     "direct",
			wantStatus: cn.APPROVED,
			wantOps:    directAdvancedLegOps(),
		},
		{
			// A hold reserves the sources only; the destination credit lands on commit, so a
			// pending transaction persists one ON_HOLD operation per SOURCE leg.
			name:       "hold opens pending and reserves every source leg",
			action:     "hold",
			wantStatus: cn.PENDING,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.ONHOLD, amount: decimal.NewFromInt(60)},
				"@srcB": {opType: cn.ONHOLD, amount: decimal.NewFromInt(40)},
			},
		},
		{
			name:       "block stamps its override on every expanded leg",
			action:     "block",
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
			action:     "unblock",
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

			resp := decodeTxResponse(t, postV2Create(t, v2App, tc.action, infra.orgID, infra.ledgerID, advancedLegV2Body, ""), nethttp.StatusCreated)
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
// full 100. `remaining` is a v1-only field — the v2 surface publishes no such field,
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
//     through a `debits` leg and settles. The v2 surface publishes no inflow/outflow action,
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
		`"debits":[{"alias":"` + externalUSDAlias + `",` + v2ScopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@alice",` + v2ScopeJSON + `,"amount":"100"}]}`

	resp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, body, ""), nethttp.StatusCreated)
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

// TestIntegration_TransactionV2Direct_ExternalAccountScalarFundsAccount used to live here,
// proving the same deposit settled identically when spelled with scalar `from`/`to` fields
// instead of the `debits`/`credits` leg arrays the test above uses. CreateTransactionV2Input
// no longer has From/To fields — the request carries ONLY leg arrays — so there is no separate
// scalar code path left to distinguish, and the test was removed rather than pointed at the
// (now identical) array mechanism the test above already covers.

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
				`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"30"}],` +
				`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"100"}]}`,
		},
		{
			// Shares: 60% + 30% = 90% of the total, so the resolved legs sum to 90.
			name: "share legs whose percentages do not sum to the whole total",
			body: `{"description":"unbalanced share legs","asset":"USD","amount":"100",` +
				`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"share":{"percentage":60}},{"alias":"@srcB",` + v2ScopeJSON + `,"share":{"percentage":30}}],` +
				`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"100"}]}`,
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

			resp := postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, tc.body, "")
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
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":60,"percentageOfPercentage":50}},` +
		`{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":70}}]}`

	resp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, body, ""), nethttp.StatusCreated)
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
// 17. SAME ACCOUNT ON BOTH SIDES OF ONE TRANSACTION. Translate names no ambiguity rule of its
//     own — the funnel's ambiguity guard is the only check that runs, and it compares the two
//     sides INDEX-POSITIONALLY, so it fires when the alias sits at the same index on both sides
//     and not otherwise.
//
//     Both outcomes are pinned. The caught row records a guarantee that no refactor should be
//     free to drop silently. The escaping row is a KNOWN-DEFECT pin: it asserts the CURRENT
//     WRONG behavior on purpose, so tightening the funnel guard goes red here and this is the
//     place to record the corrected contract.
// =============================================================================

func TestIntegration_TransactionV2_SameAccountOnBothSides(t *testing.T) {
	t.Run("same alias at the same index on both sides is rejected", func(t *testing.T) {
		// NOT parallel: process-global huma state (see file header).
		t.Setenv("ALLOW_INSECURE_TLS", "true")

		infra := setupTestInfra(t)
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		selfID, otherID := seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@self", "@other", 1000)

		v2App := buildHumaV2DirectApp(t, infra.handler)

		body := `{"description":"self transfer","asset":"USD","amount":"100","debits":[{"alias":"@self",` + v2ScopeJSON + `,"amount":"100"}],` +
			`"credits":[{"alias":"@self",` + v2ScopeJSON + `,"amount":"100"}]}`

		resp := postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, body, "")
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
		body := `{"description":"self at index one","asset":"USD","amount":"100","debits":[{"alias":"@self",` + v2ScopeJSON + `,"amount":"100"}],` +
			`"credits":[{"alias":"@other",` + v2ScopeJSON + `,"amount":"60"},{"alias":"@self",` + v2ScopeJSON + `,"amount":"40"}]}`

		resp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, body, ""), nethttp.StatusCreated)
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

	holdResp := decodeTxResponse(t, postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, advancedLegV2Body, ""), nethttp.StatusCreated)
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

// splitV2Body / splitV1Body spell the same 1->N split — one source paying 1000, two
// destinations taking an explicit 600 and 400 — in the v2 leg-array form and the v1 detailed
// form. multiSourceV2Body / multiSourceV1Body spell the mirrored N->1 shape: two sources
// contributing an explicit 600 and 400 into a single destination taking the whole 1000.
//
// All four name the aliases seedAdvancedLegBalances seeds, so the two surfaces can run the
// same intent in two ledgers and the resulting rows differ only by IDs and timestamps.
const (
	splitV2Body = `{"description":"multi-leg parity","asset":"USD","amount":"1000",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"1000"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"600"},{"alias":"@dstB",` + v2ScopeJSON + `,"amount":"400"}]}`

	splitV1Body = `{
		"description":"multi-leg parity",
		"send":{
			"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@srcA","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[
				{"accountAlias":"@dstA","amount":{"asset":"USD","value":"600"}},
				{"accountAlias":"@dstB","amount":{"asset":"USD","value":"400"}}
			]}
		}
	}`

	multiSourceV2Body = `{"description":"multi-leg parity","asset":"USD","amount":"1000",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"600"},{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"400"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"1000"}]}`

	multiSourceV1Body = `{
		"description":"multi-leg parity",
		"send":{
			"asset":"USD","value":"1000",
			"source":{"from":[
				{"accountAlias":"@srcA","amount":{"asset":"USD","value":"600"}},
				{"accountAlias":"@srcB","amount":{"asset":"USD","value":"400"}}
			]},
			"distribute":{"to":[{"accountAlias":"@dstA","amount":{"asset":"USD","value":"1000"}}]}
		}
	}`
)

// multiLegSeedAvailable is what every source account in the multi-leg parity subjects starts
// with. It is deliberately several times the largest leg so no assertion in those subjects can
// be satisfied by the balance layer refusing the movement instead of the leg resolving.
const multiLegSeedAvailable int64 = 5000

// advancedLegBalanceIDs maps the aliases advancedLegV2Body and the multi-leg parity bodies name
// to the balance IDs seedAdvancedLegBalances created for them, so a per-alias expectation table
// can be read against either ledger.
func advancedLegBalanceIDs(b advancedLegBalances) map[string]uuid.UUID {
	return map[string]uuid.UUID{"@srcA": b.srcA, "@srcB": b.srcB, "@dstA": b.dstA, "@dstB": b.dstB}
}

// assertAliasBalances asserts the final available balance of every seeded alias against the
// expectation table, and that no alias is left holding anything on hold. Every seeded alias is
// checked — including the ones no leg names — so a leg that debited the wrong account is caught
// by the untouched account rather than only by the intended one.
func assertAliasBalances(t *testing.T, db *sql.DB, ids map[string]uuid.UUID, want map[string]int64, surface string) {
	t.Helper()

	require.Len(t, want, len(ids), "the expectation table must cover every seeded alias")

	for alias, balanceID := range ids {
		wantAvailable, ok := want[alias]
		require.Truef(t, ok, "no expected balance declared for alias %s", alias)

		requireDecimalEqual(t, decimal.NewFromInt(wantAvailable),
			postgrestestutil.GetBalanceAvailable(t, db, balanceID), "%s %s available", surface, alias)
		requireDecimalEqual(t, decimal.Zero,
			postgrestestutil.GetBalanceOnHold(t, db, balanceID), "%s %s on-hold", surface, alias)
	}
}

// assertLegsSumToTotal asserts the persisted operations of a settled transaction sum to the
// declared total on BOTH sides of the entry. The funnel checks the SUBMITTED legs against the
// declared amount before it commits, which is a different claim: a leg that validates and then
// contributes no operation row leaves that check green while the persisted result is unbalanced.
// This reads the committed rows instead.
func assertLegsSumToTotal(t *testing.T, ops []operationEconomicRow, total decimal.Decimal, surface string) {
	t.Helper()

	totals := sumOperationAmountsByType(ops)

	requireDecimalEqual(t, total, totals[cn.DEBIT], "%s persisted debit total", surface)
	requireDecimalEqual(t, total, totals[cn.CREDIT], "%s persisted credit total", surface)
}

// =============================================================================
// 19. MULTI-LEG PARITY (core): the two shapes the v2 leg arrays exist for — a 1->N split and
//     an N->1 multi-source collection — are indistinguishable from the equivalent v1 detailed
//     body. Subject 12 proves the v2 surface commits a leg array correctly against absolute
//     figures; the delta here is that the SAME economic intent spelled on the released v1
//     surface lands on exactly the same operation set and the same final balances. Absolute
//     figures can be right on both surfaces and still drift apart (a leg mapped to a different
//     account type, a different rounding of the same split), which only a cross-surface
//     comparison catches.
//
//     Leg ORDER is not part of the contract on either surface, so the projection is sorted
//     before it is compared; and because each shape persists several operations sharing one
//     Type, the persisted rows are additionally checked to sum to the declared total on both
//     sides of the entry.
//
//     Each case also pins the ABSOLUTE per-operation amount on each surface before the two are
//     compared. Set equality, leg-sum and final balances are all satisfiable by a funnel writing
//     operation rows that disagree with the balances it moved — identically on both surfaces,
//     since the funnel is shared — and that divergence class is live here: subject 13 pins a leg
//     of this same funnel that validates, moves nothing and persists no row.
// =============================================================================

func TestIntegration_TransactionV2Advanced_MultiLegParityWithV1Detailed(t *testing.T) {
	cases := []struct {
		name   string
		v2Body string
		v1Body string
		// wantOps is the resolved (type, amount) of every persisted operation, keyed by alias.
		wantOps map[string]advancedLegExpectation
		// wantAvailable is the final available balance of every seeded alias, keyed by alias.
		wantAvailable map[string]int64
	}{
		{
			// 1000 leaves @srcA and is split 600/400 across two destinations; @srcB is named by
			// no leg.
			name:   "split one source across two destinations",
			v2Body: splitV2Body,
			v1Body: splitV1Body,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(1000)},
				"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(600)},
				"@dstB": {opType: cn.CREDIT, amount: decimal.NewFromInt(400)},
			},
			wantAvailable: map[string]int64{
				"@srcA": multiLegSeedAvailable - 1000,
				"@srcB": multiLegSeedAvailable,
				"@dstA": 600,
				"@dstB": 400,
			},
		},
		{
			// 600 + 400 is collected from two sources into @dstA; @dstB is named by no leg.
			name:   "collect two sources into one destination",
			v2Body: multiSourceV2Body,
			v1Body: multiSourceV1Body,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(600)},
				"@srcB": {opType: cn.DEBIT, amount: decimal.NewFromInt(400)},
				"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(1000)},
			},
			wantAvailable: map[string]int64{
				"@srcA": multiLegSeedAvailable - 600,
				"@srcB": multiLegSeedAvailable - 400,
				"@dstA": 1000,
				"@dstB": 0,
			},
		},
	}

	total := decimal.NewFromInt(1000)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// NOT parallel: process-global huma state (see file header).
			t.Setenv("ALLOW_INSECURE_TLS", "true")

			infra := setupTestInfra(t)
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

			ctx := context.Background()

			// Two ledgers under the SAME org so both surfaces can use IDENTICAL aliases and
			// starting balances; the only legitimate difference is then the ledger id plus
			// per-row IDs and timestamps.
			ledgerV1 := infra.ledgerID
			ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
			seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

			v1Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV1, multiLegSeedAvailable))
			v2Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV2, multiLegSeedAvailable))

			v1App := buildHumaTransactionApp(t, infra.handler, true)
			v2App := buildHumaV2DirectApp(t, infra.handler)

			// Each surface is fully processed (create -> drain -> assert) BEFORE the next is
			// created, because the balance-sync schedule ZSET is GLOBAL, not per-ledger:
			// draining ledgerV1 while ledgerV2's keys are still pending would claim them under
			// the wrong ledger and leave ledgerV2's cold balances stale.
			v1Resp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), tc.v1Body, ""), nethttp.StatusCreated)
			v1TxID := uuid.MustParse(v1Resp["id"].(string))
			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be APPROVED in DB")
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

			v2Resp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, ledgerV2, tc.v2Body, ""), nethttp.StatusCreated)
			v2TxID := uuid.MustParse(v2Resp["id"].(string))
			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be APPROVED in DB")
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

			v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
			v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)

			// Absolute proof first: every leg resolved to ITS OWN expected figure, and the
			// operation count is pinned to the number of legs the bodies spell, so a collapsed or
			// dropped leg fails here instead of passing a looser check.
			assertAdvancedLegOps(t, v1Ops, tc.wantOps)
			assertAdvancedLegOps(t, v2Ops, tc.wantOps)

			// Same per-account economic projection: type, asset, alias, amount, balance-after.
			assertOperationSetsEqual(t, v1Ops, v2Ops)

			// The persisted legs sum to the declared total on both sides of the entry, on both
			// surfaces.
			assertLegsSumToTotal(t, v1Ops, total, "v1")
			assertLegsSumToTotal(t, v2Ops, total, "v2")

			// Same final balances on every seeded account, read per surface against the same
			// expectation table.
			assertAliasBalances(t, infra.pgContainer.DB, v1Balances, tc.wantAvailable, "v1")
			assertAliasBalances(t, infra.pgContainer.DB, v2Balances, tc.wantAvailable, "v2")

			assert.Equal(t, "USD", v1Resp["assetCode"])
			assert.Equal(t, "USD", v2Resp["assetCode"])

			// Response deep-equal, ignoring IDs and timestamps. The `operations` array is
			// emitted in balance-internal-key ("alias#balanceKey") order, which both surfaces
			// reach through the same create funnel; every alias these bodies name is distinct, so
			// that key orders the set totally and the two arrays line up index-for-index. A body
			// naming one account twice would tie on that key and could not be compared this way.
			require.Equal(t, stripVolatile(v1Resp), stripVolatile(renameV2LegKeys(v2Resp)),
				"the v2 leg-array response must be indistinguishable from the v1 detailed equivalent (ignoring IDs/timestamps)")
		})
	}
}

// underfundedSourceLegV2Body / underfundedSourceLegV1Body spell the same N->1 collection where
// the FIRST source leg is affordable (600 against 1000 seeded) and the second is not (5000
// against 1000 seeded). The affordable leg is what makes the shape a partial-write probe: if the
// commit were not atomic across legs, @srcA would be short 600 after the rejection.
const (
	underfundedSourceLegV2Body = `{"description":"underfunded source leg","asset":"USD","amount":"5600",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"600"},{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"5000"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"amount":"5600"}]}`

	underfundedSourceLegV1Body = `{
		"description":"underfunded source leg",
		"send":{
			"asset":"USD","value":"5600",
			"source":{"from":[
				{"accountAlias":"@srcA","amount":{"asset":"USD","value":"600"}},
				{"accountAlias":"@srcB","amount":{"asset":"USD","value":"5000"}}
			]},
			"distribute":{"to":[{"accountAlias":"@dstA","amount":{"asset":"USD","value":"5600"}}]}
		}
	}`
)

// =============================================================================
// 20. ONE UNDERFUNDED SOURCE LEG FAILS THE WHOLE TRANSACTION: in a multi-source collection
//     where one leg is affordable and another is not, the balance commit rejects the request as
//     a business error (ErrInsufficientFunds / 0018 -> 422) and NOTHING is written — no
//     transaction row, and in particular the AFFORDABLE leg's account is left exactly as
//     seeded. Subject 5 makes the same claim for a single-source transfer, where "all or
//     nothing" and "the only leg was refused" are the same observation; two source legs, one of
//     them affordable, separate the two.
//
//     The balance the atomic commit mutates lives in Redis and reaches PostgreSQL only through
//     the balance-sync drain, so each surface is drained after its rejection and before the cold
//     rows are read. Undrained, the untouched-balance assertion reads the seeded rows on every
//     rejection path and says nothing about what the hot layer did.
//
//     Both surfaces are asserted, because the leg arrays would still be a regression if v2
//     accepted a shape v1 refuses (or refused it with a different code).
// =============================================================================

func TestIntegration_TransactionV2Advanced_UnderfundedSourceLegRejectsWholeTransaction(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Distinct ledgers under the same org, so each surface's "no transaction persisted" claim
	// is scoped to its own ledger.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	// Both sources seeded with 1000: @srcA's 600 leg fits, @srcB's 5000 leg does not.
	v1Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV1, 1000))
	v2Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV2, 1000))

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	untouched := map[string]int64{"@srcA": 1000, "@srcB": 1000, "@dstA": 0, "@dstB": 0}

	for _, surface := range []struct {
		name     string
		app      *fiber.App
		url      string
		body     string
		ledgerID uuid.UUID
		balances map[string]uuid.UUID
	}{
		{"v1", v1App, v1JSONURL(infra.orgID, ledgerV1), underfundedSourceLegV1Body, ledgerV1, v1Balances},
		{"v2", v2App, v2CreateURL("direct"), v2ScopedBody(underfundedSourceLegV2Body, infra.orgID, ledgerV2), ledgerV2, v2Balances},
	} {
		// A subtest per surface: the helpers below are require-based, so a v1 failure would
		// otherwise end the test before v2 ran at all.
		t.Run(surface.name, func(t *testing.T) {
			resp := postTransaction(t, surface.app, surface.url, surface.body, "")
			body := drainBody(t, resp)

			assert.Equal(t, nethttp.StatusUnprocessableEntity, resp.StatusCode,
				"%s: an underfunded source leg is a business error (ErrInsufficientFunds / 0018) -> 422; body: %s", surface.name, string(body))
			requireProblemCode(t, body, "0018")

			assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, surface.ledgerID),
				"%s: an underfunded source leg must not persist a transaction", surface.name)

			// Flush this surface's hot balances into PostgreSQL, so the rows read below carry
			// whatever the atomic commit left behind rather than the seeded values.
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, surface.ledgerID)

			// No partial write: the AFFORDABLE leg's account still holds everything it was seeded
			// with, and the destination received nothing.
			assertAliasBalances(t, infra.pgContainer.DB, surface.balances, untouched, surface.name)
		})
	}
}

// The share bodies below spell the same intent on both surfaces: one source paying the whole
// 1000, and two destinations taking their value as a percentage of it instead of as an absolute
// amount. Translate carries a v2 leg's share expression forward untouched, so the number each
// leg resolves to is produced by the same funnel arithmetic v1 reaches — which is what these
// pairs are here to demonstrate against the released surface rather than against a hand-computed
// figure.
//
// shareSplit* is the plain two-factor-free split (70/30). sharePop* additionally exercises
// percentageOfPercentage on ONE of the two legs: @dstA narrows 80% by 25% (20% of the total) and
// @dstB spells 80% with the field OMITTED, so the same body pins both the narrowing and the
// omitted-field behaviour. All four factors divide 1000 exactly, so no assertion depends on how
// the decimal divide rounds.
const (
	shareSplitV2Body = `{"description":"share leg parity","asset":"USD","amount":"1000",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"1000"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":70}},{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":30}}]}`

	shareSplitV1Body = `{
		"description":"share leg parity",
		"send":{
			"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@srcA","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[
				{"accountAlias":"@dstA","share":{"percentage":70}},
				{"accountAlias":"@dstB","share":{"percentage":30}}
			]}
		}
	}`

	sharePopV2Body = `{"description":"share leg parity","asset":"USD","amount":"1000",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"1000"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":80,"percentageOfPercentage":25}},` +
		`{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":80}}]}`

	sharePopV1Body = `{
		"description":"share leg parity",
		"send":{
			"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@srcA","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[
				{"accountAlias":"@dstA","share":{"percentage":80,"percentageOfPercentage":25}},
				{"accountAlias":"@dstB","share":{"percentage":80}}
			]}
		}
	}`
)

// =============================================================================
// 21. SHARE-LEG PARITY (core): a destination side whose legs express their value as a share of
//     the transaction total resolves to the same per-operation figures on the v2 leg arrays as
//     on the v1 detailed body — with and without the second factor. Subject 12 already commits a
//     50/50 share body through v2 against absolute figures; the delta here is the cross-surface
//     comparison, plus percentageOfPercentage, which subject 12 never spells.
//
//     Each case pins the RESOLVED per-operation amount on each surface before comparing the two,
//     because a split can be identical on both surfaces and still be wrong — an ignored second
//     factor, or a whole share landing on one leg, produces matching sets that a
//     cross-surface-only comparison would accept.
// =============================================================================

func TestIntegration_TransactionV2Advanced_ShareLegParityWithV1Detailed(t *testing.T) {
	cases := []struct {
		name   string
		v2Body string
		v1Body string
		// wantOps is the resolved (type, amount) of every persisted operation, keyed by alias.
		wantOps map[string]advancedLegExpectation
		// wantAvailable is the final available balance of every seeded alias, keyed by alias.
		wantAvailable map[string]int64
	}{
		{
			// 1000 leaves @srcA and is split 70/30 across two share legs; @srcB is named by no
			// leg. Any other in-bounds pair adding to 100 — 71/29 — closes the total just as
			// well, so the per-leg figures are pinned rather than only their sum.
			name:   "split one source across two share legs",
			v2Body: shareSplitV2Body,
			v1Body: shareSplitV1Body,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(1000)},
				"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(700)},
				"@dstB": {opType: cn.CREDIT, amount: decimal.NewFromInt(300)},
			},
			wantAvailable: map[string]int64{
				"@srcA": multiLegSeedAvailable - 1000,
				"@srcB": multiLegSeedAvailable,
				"@dstA": 700,
				"@dstB": 300,
			},
		},
		{
			// @dstA: 80% narrowed by 25% -> 20% of 1000 = 200. @dstB: 80% with the second factor
			// omitted -> 800. The two close the total only if the narrowing is applied to the
			// first leg AND not applied to the second, so a surface that ignored the field, or
			// that read an omitted field as a zero share, fails the funnel's total check instead
			// of producing a subtly wrong split.
			name:   "share narrowed by percentageOfPercentage on one leg only",
			v2Body: sharePopV2Body,
			v1Body: sharePopV1Body,
			wantOps: map[string]advancedLegExpectation{
				"@srcA": {opType: cn.DEBIT, amount: decimal.NewFromInt(1000)},
				"@dstA": {opType: cn.CREDIT, amount: decimal.NewFromInt(200)},
				"@dstB": {opType: cn.CREDIT, amount: decimal.NewFromInt(800)},
			},
			wantAvailable: map[string]int64{
				"@srcA": multiLegSeedAvailable - 1000,
				"@srcB": multiLegSeedAvailable,
				"@dstA": 200,
				"@dstB": 800,
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

			// Two ledgers under the SAME org so both surfaces can use IDENTICAL aliases and
			// starting balances; the only legitimate difference is then the ledger id plus
			// per-row IDs and timestamps.
			ledgerV1 := infra.ledgerID
			ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
			seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

			v1Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV1, multiLegSeedAvailable))
			v2Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV2, multiLegSeedAvailable))

			v1App := buildHumaTransactionApp(t, infra.handler, true)
			v2App := buildHumaV2DirectApp(t, infra.handler)

			// Each surface is fully processed (create -> drain -> assert) BEFORE the next is
			// created, because the balance-sync schedule ZSET is GLOBAL, not per-ledger:
			// draining ledgerV1 while ledgerV2's keys are still pending would claim them under
			// the wrong ledger and leave ledgerV2's cold balances stale.
			v1Resp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, ledgerV1), tc.v1Body, ""), nethttp.StatusCreated)
			v1TxID := uuid.MustParse(v1Resp["id"].(string))
			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v1TxID), "v1 transaction should be APPROVED in DB")
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV1)

			v2Resp := decodeTxResponse(t, postV2Create(t, v2App, "direct", infra.orgID, ledgerV2, tc.v2Body, ""), nethttp.StatusCreated)
			v2TxID := uuid.MustParse(v2Resp["id"].(string))
			assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, v2TxID), "v2 transaction should be APPROVED in DB")
			drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, ledgerV2)

			v1Ops := fetchOperationRows(t, infra.pgContainer.DB, v1TxID)
			v2Ops := fetchOperationRows(t, infra.pgContainer.DB, v2TxID)

			// Absolute proof first: every share leg resolved to ITS OWN expected figure, and the
			// operation count is pinned to the number of legs the bodies spell.
			assertAdvancedLegOps(t, v1Ops, tc.wantOps)
			assertAdvancedLegOps(t, v2Ops, tc.wantOps)

			// Same per-account economic projection: type, asset, alias, amount, balance-after.
			assertOperationSetsEqual(t, v1Ops, v2Ops)

			// Same final balances on every seeded account, including the source no leg names.
			assertAliasBalances(t, infra.pgContainer.DB, v1Balances, tc.wantAvailable, "v1")
			assertAliasBalances(t, infra.pgContainer.DB, v2Balances, tc.wantAvailable, "v2")

			assert.Equal(t, "USD", v1Resp["assetCode"])
			assert.Equal(t, "USD", v2Resp["assetCode"])

			// Response deep-equal, ignoring IDs and timestamps. The `operations` array is emitted
			// in balance-internal-key ("alias#balanceKey") order, which both surfaces reach
			// through the same create funnel; every alias these bodies name is distinct, so that
			// key orders the set totally and the two arrays line up index-for-index.
			require.Equal(t, stripVolatile(v1Resp), stripVolatile(renameV2LegKeys(v2Resp)),
				"the v2 share-leg response must be indistinguishable from the v1 detailed equivalent (ignoring IDs/timestamps)")
		})
	}
}

// shareShortV2Body / shareShortV1Body spell a destination side whose shares add up to 90% of the
// declared 1000, on each surface. 70 and 20 are each individually within the bound the v2 field
// publishes, so nothing per-field can refuse this body — whether a side's legs close the total is
// a whole-body property, and the funnel's own comparison is the only thing that owns it.
const (
	shareShortV2Body = `{"description":"share legs short of the total","asset":"USD","amount":"1000",` +
		`"debits":[{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"1000"}],` +
		`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":70}},{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":20}}]}`

	shareShortV1Body = `{
		"description":"share legs short of the total",
		"send":{
			"asset":"USD","value":"1000",
			"source":{"from":[{"accountAlias":"@srcA","amount":{"asset":"USD","value":"1000"}}]},
			"distribute":{"to":[
				{"accountAlias":"@dstA","share":{"percentage":70}},
				{"accountAlias":"@dstB","share":{"percentage":20}}
			]}
		}
	}`
)

// =============================================================================
// 22. SHARE LEGS THAT DO NOT CLOSE THE TOTAL ARE REFUSED WITH NO LEDGER EFFECT: shares summing
//     to 90% of the declared amount are rejected by the funnel's total comparison
//     (ErrTransactionValueMismatch / 0073 -> 422) with nothing persisted, on BOTH surfaces. The
//     canonical code is asserted and not merely the status, because the v2 surface answers with
//     several distinct 4xx layers and only the code says which one refused: a shape or bound
//     rejection here would mean the funnel comparison was never reached, and the claim being made
//     is precisely that it was.
// =============================================================================

func TestIntegration_TransactionV2Advanced_ShareLegsNotClosingTotalRejected(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	// Distinct ledgers under the same org, so each surface's "no transaction persisted" claim is
	// scoped to its own ledger.
	ledgerV1 := infra.ledgerID
	ledgerV2 := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, ledgerV2)

	v1Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV1, multiLegSeedAvailable))
	v2Balances := advancedLegBalanceIDs(seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, ledgerV2, multiLegSeedAvailable))

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2DirectApp(t, infra.handler)

	untouched := map[string]int64{
		"@srcA": multiLegSeedAvailable,
		"@srcB": multiLegSeedAvailable,
		"@dstA": 0,
		"@dstB": 0,
	}

	for _, surface := range []struct {
		name     string
		app      *fiber.App
		url      string
		body     string
		ledgerID uuid.UUID
		balances map[string]uuid.UUID
	}{
		{"v1", v1App, v1JSONURL(infra.orgID, ledgerV1), shareShortV1Body, ledgerV1, v1Balances},
		{"v2", v2App, v2CreateURL("direct"), v2ScopedBody(shareShortV2Body, infra.orgID, ledgerV2), ledgerV2, v2Balances},
	} {
		// A subtest per surface: the helpers below are require-based, so a v1 failure would
		// otherwise end the test before v2 ran at all.
		t.Run(surface.name, func(t *testing.T) {
			resp := postTransaction(t, surface.app, surface.url, surface.body, "")
			body := drainBody(t, resp)

			assert.Equal(t, nethttp.StatusUnprocessableEntity, resp.StatusCode,
				"%s: shares that do not close the total are a business error (ErrTransactionValueMismatch / 0073) -> 422; body: %s", surface.name, string(body))
			requireProblemCode(t, body, "0073")

			assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, surface.ledgerID),
				"%s: shares short of the declared total must not persist a transaction", surface.name)

			assertAliasBalances(t, infra.pgContainer.DB, surface.balances, untouched, surface.name)
		})
	}
}

// advancedLegPermutedV2Body is advancedLegV2Body with its two source legs written in the
// opposite order. Economically it is the same transaction; as bytes it is a different request.
const advancedLegPermutedV2Body = `{"description":"v2 advanced multi-leg","asset":"USD","amount":"100",` +
	`"debits":[{"alias":"@srcB",` + v2ScopeJSON + `,"amount":"40"},{"alias":"@srcA",` + v2ScopeJSON + `,"amount":"60"}],` +
	`"credits":[{"alias":"@dstA",` + v2ScopeJSON + `,"share":{"percentage":50}},{"alias":"@dstB",` + v2ScopeJSON + `,"share":{"percentage":50}}]}`

// responseOperationsByAliasAndType projects a transaction RESPONSE's operations array into a map
// keyed by "<accountAlias>/<type>", each value the operation object with its identity, timestamp
// and route keys stripped by stripVolatile — so what remains is the economic content of the leg.
// It is the response-JSON counterpart of operationsByAliasAndType, which does the same join over
// the persisted rows; the two cannot share an implementation because they read different shapes.
//
// Keying by (alias, type) rather than comparing the arrays index-for-index keeps the comparison
// independent of the order the operations are serialised in.
func responseOperationsByAliasAndType(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()

	ops, ok := resp["operations"].([]any)
	require.True(t, ok, "a committed transaction response must carry an operations array")
	require.NotEmpty(t, ops, "a committed transaction response must carry at least one operation")

	out := make(map[string]any, len(ops))

	for _, raw := range ops {
		op, isObject := raw.(map[string]any)
		require.True(t, isObject, "each response operation must decode as an object")

		alias, hasAlias := op["accountAlias"].(string)
		require.True(t, hasAlias, "each response operation must carry a string accountAlias")

		opType, hasType := op["type"].(string)
		require.True(t, hasType, "each response operation must carry a string type")

		key := alias + "/" + opType

		_, dup := out[key]
		require.Falsef(t, dup, "two response operations share alias %s and type %s", alias, opType)

		out[key] = stripVolatile(op)
	}

	return out
}

// =============================================================================
// 23. ADVANCED (LEG-ARRAY) IDEMPOTENCY OVER REAL REDIS: an advanced body replayed under the
//     SAME X-Idempotency key returns the FIRST transaction — with its FULL per-leg projection,
//     not just its id — and creates no second transaction. The per-leg claim is what separates
//     a multi-leg replay from a scalar one: a replay that re-resolved the legs, or resolved only
//     some of them, would still satisfy an id-only assertion while handing the client a different
//     economic answer under the same id. A client-supplied key REPLACES the body hash as the slot
//     identity and the replay branch compares no hash, so this subject says nothing about how the
//     hash is built — only the no-key subjects reach that.
//
//     A second subject records the present behaviour of the no-key hash: it is taken over the RAW
//     body bytes, so permuting the leg array — the same transaction economically — is a different
//     byte sequence and lands in its own slot. That follows from where the hash is taken; it is
//     NOT a semantic guarantee that leg order is part of a request's identity.
// =============================================================================

func TestIntegration_TransactionV2Advanced_Idempotency(t *testing.T) {
	t.Run("replay returns the first transaction leg for leg", func(t *testing.T) {
		// NOT parallel: process-global huma state (see file header).
		t.Setenv("ALLOW_INSECURE_TLS", "true")

		infra := setupTestInfra(t)
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		ctx := context.Background()

		balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

		v2App := buildHumaV2DirectApp(t, infra.handler)
		url := v2CreateURL("direct")
		idempotencyKey := uuid.NewString()

		first := postTransaction(t, v2App, url, v2ScopedBody(advancedLegV2Body, infra.orgID, infra.ledgerID), idempotencyKey)
		firstResult := decodeTxResponse(t, first, nethttp.StatusCreated)
		assert.Equal(t, "false", first.Header.Get("X-Idempotency-Replayed"), "first advanced call must not be a replay")

		firstTxID := firstResult["id"].(string)

		// Wait for the async idempotency-value store before replaying (see helper doc).
		waitForIdempotencyStored(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, idempotencyKey)

		second := postTransaction(t, v2App, url, v2ScopedBody(advancedLegV2Body, infra.orgID, infra.ledgerID), idempotencyKey)
		secondResult := decodeTxResponse(t, second, nethttp.StatusCreated)

		assert.Equal(t, "true", second.Header.Get("X-Idempotency-Replayed"), "second identical advanced call must be a replay")
		assert.Equal(t, firstTxID, secondResult["id"].(string), "replay must return the FIRST transaction's id")

		// All four legs must come back with identical economic content, joined by (alias, type) so
		// serialisation order does not enter the comparison. Read before anything else touches the
		// two response maps, because stripVolatile mutates in place.
		firstProjection := responseOperationsByAliasAndType(t, firstResult)
		secondProjection := responseOperationsByAliasAndType(t, secondResult)

		// The projection LENGTH is pinned to the leg count first: two responses agreeing with each
		// other on a partial set of legs would satisfy the comparison below on its own.
		require.Len(t, firstProjection, len(directAdvancedLegOps()), "the create response must project every leg the body spells")
		require.Equal(t, firstProjection, secondProjection,
			"the replay must return the first transaction's per-leg projection, leg for leg")

		assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
			"an idempotent advanced replay must NOT create a second transaction")

		// The ledger holds exactly the first transaction's four legs, and the money moved once.
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		assertAdvancedLegOps(t, fetchOperationRows(t, infra.pgContainer.DB, uuid.MustParse(firstTxID)), directAdvancedLegOps())
		assertAliasBalances(t, infra.pgContainer.DB, advancedLegBalanceIDs(balances), map[string]int64{
			"@srcA": 940,
			"@srcB": 960,
			"@dstA": 50,
			"@dstB": 50,
		}, "advanced replay")
	})

	t.Run("permuting the leg array is a different no-key slot", func(t *testing.T) {
		// NOT parallel: process-global huma state (see file header).
		t.Setenv("ALLOW_INSECURE_TLS", "true")

		infra := setupTestInfra(t)
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		ctx := context.Background()

		// Both sources are seeded well above the sum of the two commits below, so neither
		// transaction can be turned back by the funds guard.
		balances := seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

		v2App := buildHumaV2DirectApp(t, infra.handler)
		url := v2CreateURL("direct")

		// No X-Idempotency header on either call, so each derives its key from its own body bytes.
		firstResult := decodeTxResponse(t, postTransaction(t, v2App, url, v2ScopedBody(advancedLegV2Body, infra.orgID, infra.ledgerID), ""), nethttp.StatusCreated)

		permuted := postTransaction(t, v2App, url, v2ScopedBody(advancedLegPermutedV2Body, infra.orgID, infra.ledgerID), "")
		permutedResult := decodeTxResponse(t, permuted, nethttp.StatusCreated)

		assert.Equal(t, "false", permuted.Header.Get("X-Idempotency-Replayed"),
			"the permuted body hashes to its own slot, so it must not replay the original")
		assert.NotEqual(t, firstResult["id"].(string), permutedResult["id"].(string),
			"a permuted leg array is a distinct byte sequence and therefore a distinct transaction")
		assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
			"both bodies must commit: the raw-body hash does not canonicalise leg order")

		// The two commits are economically identical, so every account moved twice by the same
		// leg — the permutation changed the request identity, not the accounting.
		drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

		assertAliasBalances(t, infra.pgContainer.DB, advancedLegBalanceIDs(balances), map[string]int64{
			"@srcA": 880,
			"@srcB": 920,
			"@dstA": 100,
			"@dstB": 100,
		}, "permuted advanced")
	})
}

// =============================================================================
// 24. ADVANCED (LEG-ARRAY) DIRECT↔HOLD NO-KEY CROSS-DEDUP: the v2 action is carried by the
//     ENDPOINT, not the body, so a byte-identical ADVANCED body posted to /direct and then to
//     /hold in the same org/ledger with NO X-Idempotency header must not cross-replay. Subject 9
//     makes this claim for the scalar body; the delta here is that the leg-array spelling — a
//     longer, structurally richer byte sequence feeding the same hash source — keeps the two
//     actions in distinct slots, so two DISTINCT transactions land with their own statuses.
// =============================================================================

func TestIntegration_TransactionV2Advanced_DirectHoldNoKeyCrossDedup(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	// Each source is seeded well above its direct leg plus its hold reservation, so both actions
	// clear the funds guard and reach persistence.
	seedAdvancedLegBalances(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	directResp := postV2Create(t, v2App, "direct", infra.orgID, infra.ledgerID, advancedLegV2Body, "")
	directResult := decodeTxResponse(t, directResp, nethttp.StatusCreated)
	assert.Equal(t, "false", directResp.Header.Get("X-Idempotency-Replayed"), "the advanced direct create must not be a replay")

	directTxID := uuid.MustParse(directResult["id"].(string))

	holdResp := postV2Create(t, v2App, "hold", infra.orgID, infra.ledgerID, advancedLegV2Body, "")
	holdResult := decodeTxResponse(t, holdResp, nethttp.StatusCreated)
	assert.Equal(t, "false", holdResp.Header.Get("X-Idempotency-Replayed"),
		"the advanced hold create must NOT replay the advanced direct transaction (distinct action identity, distinct slot)")

	holdTxID := uuid.MustParse(holdResult["id"].(string))

	assert.NotEqual(t, directTxID, holdTxID,
		"an identical advanced body posted to /direct and /hold must create two DISTINCT transactions")

	// Distinct statuses: the direct settles immediately, the hold stays PENDING.
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, directTxID),
		"the advanced direct transaction should be APPROVED, not PENDING")
	assert.Equal(t, cn.PENDING, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, holdTxID),
		"the advanced hold transaction should be PENDING")

	assert.Equal(t, 2, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID),
		"an advanced direct + hold with an identical no-key body must persist two transactions, not replay one")
}

// =============================================================================
// 20. BODY SCOPE DECIDES THE LEDGER: the v2 create path names no organization and no
//     ledger, so the pair the request body states is the only thing that can decide where
//     the transaction lands. Two ledgers are seeded with the SAME aliases and the SAME
//     starting balances, so nothing but the scope in the body distinguishes them — and the
//     transaction must appear in the one the body named and nowhere else.
// =============================================================================

func TestIntegration_TransactionV2Direct_BodyScopeDecidesTheLedger(t *testing.T) {
	// NOT parallel: process-global huma state (see file header).
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and refuses
	// plaintext unless ALLOW_INSECURE_TLS=true.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Two ledgers under one organization, seeded identically. The named one receives the
	// transaction; the other is the control.
	unnamedLedger := infra.ledgerID
	namedLedger := uuid.Must(libCommons.GenerateUUIDv7())
	seedLedgerSettings(t, infra.pgContainer.DB, infra.orgID, namedLedger)

	unnamedSrc, unnamedDst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, unnamedLedger, "@src", "@dst", 1000)
	namedSrc, namedDst := seedTransfer(t, infra.pgContainer.DB, infra.orgID, namedLedger, "@src", "@dst", 1000)

	v2App := buildHumaV2DirectApp(t, infra.handler)

	resp := decodeTxResponse(t,
		postV2Create(t, v2App, "direct", infra.orgID, namedLedger, equivalentV2Body, ""),
		nethttp.StatusCreated)

	txID := uuid.MustParse(resp["id"].(string))
	assert.Equal(t, cn.APPROVED, postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID),
		"the transaction must settle in the ledger the body named")

	assert.Equal(t, namedLedger.String(), resp["ledgerId"],
		"the response must report the ledger the body named")
	assert.Equal(t, infra.orgID.String(), resp["organizationId"],
		"the response must report the organization the body named")

	assert.Equal(t, 1, countTransactionsInLedger(t, infra.pgContainer.DB, namedLedger),
		"the named ledger must hold the transaction")
	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, unnamedLedger),
		"no other ledger may hold a transaction the body never named")

	// Balances move in the named ledger only. Both ledgers are drained so an effect landing in
	// the wrong one cannot hide behind an unflushed hot balance.
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, namedLedger)
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, unnamedLedger)

	requireDecimalEqual(t, decimal.NewFromInt(900), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, namedSrc),
		"the named ledger's source must be debited")
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, namedDst),
		"the named ledger's destination must be credited")
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, unnamedSrc),
		"a ledger the body never named must be untouched")
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, unnamedDst),
		"a ledger the body never named must be untouched")
}
