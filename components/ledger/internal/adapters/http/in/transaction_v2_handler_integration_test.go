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

	sourceAccountID := uuid.Must(libCommons.GenerateUUIDv7())
	destAccountID := uuid.Must(libCommons.GenerateUUIDv7())

	srcParams := postgrestestutil.DefaultBalanceParams()
	srcParams.Alias = sourceAlias
	srcParams.AssetCode = "USD"
	srcParams.Available = decimal.NewFromInt(available)
	srcParams.OnHold = decimal.Zero
	sourceBalanceID = postgrestestutil.CreateTestBalance(t, db, orgID, ledgerID, sourceAccountID, srcParams)

	dstParams := postgrestestutil.DefaultBalanceParams()
	dstParams.Alias = destAlias
	dstParams.AssetCode = "USD"
	dstParams.Available = decimal.Zero
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

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			// `from` is validate:"required" on CreateTransactionV2Input; the imperative
			// DecodeAndValidate surfaces a ValidationError -> canonical 400 before the funnel.
			name:       "missing required from field",
			body:       `{"asset":"USD","amount":"100","to":"@dst"}`,
			wantStatus: nethttp.StatusBadRequest,
		},
		{
			name:       "missing required asset field",
			body:       `{"amount":"100","from":"@src","to":"@dst"}`,
			wantStatus: nethttp.StatusBadRequest,
		},
		{
			name:       "malformed json body",
			body:       `{not-json`,
			wantStatus: nethttp.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postTransaction(t, v2App, url, tc.body, "")
			_ = drainBody(t, resp)
			assert.Equal(t, tc.wantStatus, resp.StatusCode, "%s should be rejected at the decode boundary", tc.name)
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

// v1CommitURL builds the concrete v1 commit path for a pending transaction. v2 has no
// commit op yet (Phase 3), so a v2-held transaction settles through the v1 commit endpoint.
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
// 7. HOLD COMMIT (lifecycle): a v2-held transaction is committable through the existing
//    v1 commit endpoint (v2 commit is Phase 3). Commit settles the hold: the source
//    on-hold releases and the destination credit applies, the transaction flips to
//    APPROVED, and the full ON_HOLD + DEBIT + CREDIT operation set is present.
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

	// Full lifecycle operation set: 1 ON_HOLD (hold) + 1 DEBIT (release) + 1 CREDIT (apply).
	assert.Equal(t, 3, postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID),
		"committed hold should carry exactly 3 operations (ON_HOLD + DEBIT + CREDIT)")
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
