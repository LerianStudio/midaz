// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the happy-path behavioral proof for the three /v2 transaction mirror ops that carry
// no dedicated create/lifecycle wire shape — GetTransactionV2Huma, GetAllTransactionsV2Huma, and
// UpdateTransactionV2Huma (transaction_v2_mirror_handler.go). The unit suite only reaches these on
// the invalid-path-ID guard branch; here they run against a LIVE app backed by testcontainers, over
// a transaction seeded through a v1 create so the seed path is unaffected by the v2 field removals.
//
// The seam under proof is newTransactionV2 / newOperationV2: the /v2 read + update responses must
// DROP the deprecated fields (transaction-level chartOfAccountsGroupName and route, operation-level
// chartOfAccounts and route) and spell the leg lists debit/credit instead of source/destination,
// while retaining routeId. Field absence is asserted DIRECTLY on the parsed v2 JSON — NOT through
// stripVolatile, which deletes `route` from both surfaces and would mask exactly the regression
// these cases guard. The v1 create response is captured as an anchor and asserted to CARRY those
// same keys, so each absence assertion is a real differential and not a check over a field that was
// never emitted on either surface.
//
// NOT parallel: buildHumaTransactionApp / buildHumaV2MirrorApp call libProblem.Install()
// (process-global huma.NewError hook) and Huma validation uses process-global sync.Pools; every
// test here stays sequential (see the create/hold file header for the full rationale).

// buildHumaV2MirrorApp mounts the three /v2 transaction mirror ops (get-by-id, list, PATCH update)
// through the SAME production seam (RegisterTransactionMirrorV2RoutesToApp) the unified server uses,
// on a fresh Fiber app + its own /v2 Huma contract. It mirrors buildHumaV2DirectApp — problem.Install()
// before any huma.Register, WithRecover first so an unwired-repo panic unwinds to a 500, and the
// ledger schema namer installed before registration (the v2 body nests operation.Operation and
// clashes on the bare names without it) — but wires the mirror registrar rather than the create/
// lifecycle one, and disables auth so requests reach the terminal.
func buildHumaV2MirrorApp(t *testing.T, handler *TransactionHandler) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	app.Use(pkgHTTP.WithRecover(pkgHTTP.WithRecoverLogger(&libLog.GoLogger{})))

	libProblem.Install()

	apiV2 := app.Group("/v2")

	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-test-v2-mirror", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionMirrorV2RoutesToApp(apiV2, humaAPI, &middleware.AuthClient{Enabled: false}, handler, nil)

	return app
}

// v2TxByIDURL / v2TxListURL build the concrete /v2 mirror read paths. Both are GROUP-relative on the
// contract but concrete on the wire (the /v2 prefix + the org/ledger scope in the path).
func v2TxByIDURL(orgID, ledgerID, txID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions/" + txID.String()
}

func v2TxListURL(orgID, ledgerID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/transactions"
}

// getV2 issues a GET to the given app/url and returns the raw response.
func getV2(t *testing.T, app *fiber.App, url string) *nethttp.Response {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodGet, url, nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "GET request should not fail at the transport layer")

	return resp
}

// patchV2 issues a PATCH to the given app/url with a JSON body and returns the raw response.
func patchV2(t *testing.T, app *fiber.App, url, body string) *nethttp.Response {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodPatch, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "PATCH request should not fail at the transport layer")

	return resp
}

// assertV1TransactionCarriesDeprecated anchors the differential: the v1 read surface still emits the
// deprecated transaction- and operation-level fields (they are non-omitempty strings on the v1 wire
// shapes), plus source/destination. Proving they are PRESENT on v1 is what makes each v2 absence
// assertion below a real regression guard rather than a check over a field nothing emits.
func assertV1TransactionCarriesDeprecated(t *testing.T, tx map[string]any, label string) {
	t.Helper()

	assert.Containsf(t, tx, "chartOfAccountsGroupName", "%s: v1 anchor must carry chartOfAccountsGroupName", label)
	assert.Containsf(t, tx, "route", "%s: v1 anchor must carry the top-level route", label)
	assert.Containsf(t, tx, "source", "%s: v1 anchor must carry the source leg list", label)
	assert.Containsf(t, tx, "destination", "%s: v1 anchor must carry the destination leg list", label)

	ops := v2OperationsOf(t, tx, label)
	require.NotEmptyf(t, ops, "%s: v1 anchor must carry operations so the op-level differential is real", label)

	for i, op := range ops {
		assert.Containsf(t, op, "chartOfAccounts", "%s: v1 anchor operation[%d] must carry chartOfAccounts", label, i)
		assert.Containsf(t, op, "route", "%s: v1 anchor operation[%d] must carry the operation-level route", label, i)
	}
}

// assertV2TransactionOmitsDeprecated is the core regression guard: a raw /v2 transaction object must
// DROP chartOfAccountsGroupName and the top-level route, DROP each operation's chartOfAccounts and
// route, and spell the leg lists debit/credit — never source/destination. Every check reads the
// parsed map DIRECTLY, so stripVolatile's route deletion cannot hide a leak. requireOps demands the
// operation array be populated (the two reads project it); the PATCH update core does NOT project
// operations, so its caller passes false and the op-level checks run over the empty set.
func assertV2TransactionOmitsDeprecated(t *testing.T, tx map[string]any, label string, requireOps bool) {
	t.Helper()

	assert.NotContainsf(t, tx, "chartOfAccountsGroupName", "%s: v2 transaction must drop chartOfAccountsGroupName", label)
	assert.NotContainsf(t, tx, "route", "%s: v2 transaction must drop the top-level route", label)
	assert.Containsf(t, tx, "debit", "%s: v2 transaction must carry the renamed debit leg list", label)
	assert.Containsf(t, tx, "credit", "%s: v2 transaction must carry the renamed credit leg list", label)
	assert.NotContainsf(t, tx, "source", "%s: v2 transaction must not carry the v1 source key", label)
	assert.NotContainsf(t, tx, "destination", "%s: v2 transaction must not carry the v1 destination key", label)

	ops := v2OperationsOf(t, tx, label)
	if requireOps {
		require.NotEmptyf(t, ops, "%s: v2 transaction must carry operations so the op-level absence checks are not vacuous", label)
	}

	for i, op := range ops {
		assert.NotContainsf(t, op, "chartOfAccounts", "%s: v2 operation[%d] must drop chartOfAccounts", label, i)
		assert.NotContainsf(t, op, "route", "%s: v2 operation[%d] must drop the operation-level route", label, i)
	}
}

// v2OperationsOf reads the operations array off a raw transaction map as a slice of objects. A nil
// or absent operations value returns an empty slice (the PATCH update response projects no
// operations); a present-but-non-array value is a decode error and fails the test.
func v2OperationsOf(t *testing.T, tx map[string]any, label string) []map[string]any {
	t.Helper()

	value, present := tx["operations"]
	if !present || value == nil {
		return nil
	}

	raw, ok := value.([]any)
	require.Truef(t, ok, "%s: operations must decode as an array; got %T", label, value)

	out := make([]map[string]any, 0, len(raw))

	for i, item := range raw {
		op, ok := item.(map[string]any)
		require.Truef(t, ok, "%s: operation[%d] must decode as an object", label, i)

		out = append(out, op)
	}

	return out
}

// TestIntegration_TransactionV2MirrorReadUpdate_OmitsDeprecatedFields seeds ONE settled transaction
// through a v1 create, then exercises the three /v2 mirror ops (get-by-id, list, PATCH) against a
// live app and proves each drops the deprecated fields on the real v2 response. Sequential subtests
// share the single seeded transaction (the v1 anchor is captured once); no t.Parallel — integration
// tests share container state and this package's Huma state is process-global.
func TestIntegration_TransactionV2MirrorReadUpdate_OmitsDeprecatedFields(t *testing.T) {
	// The postgres client constructor enforces TLS by the ENV_NAME security tier and refuses
	// plaintext unless ALLOW_INSECURE_TLS=true (mirrors the sibling v2 integration files).
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	// Seed a settled 100 USD @src->@dst transfer through the v1 /json create so the seed path is
	// untouched by the v2 removals. The create is synchronous (async off), so the transaction is
	// in Postgres immediately — the list DB query below finds it, and the read/update cores resolve
	// it. Drain the balance-sync effect so the shared global ZSET does not leak into later reads.
	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2MirrorApp(t, infra.handler)

	v1CreateResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, infra.ledgerID), equivalentV1Body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(v1CreateResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	// Anchor: the v1 create response carries exactly the fields the v2 surface must drop, so every
	// v2 absence assertion below is a genuine differential.
	assertV1TransactionCarriesDeprecated(t, v1CreateResp, "v1 create anchor")

	t.Run("get_by_id", func(t *testing.T) {
		resp := getV2(t, v2App, v2TxByIDURL(infra.orgID, infra.ledgerID, txID))
		got := decodeTxResponse(t, resp, nethttp.StatusOK)

		assert.Equal(t, txID.String(), got["id"], "get-by-id must return the seeded transaction")
		assert.Equal(t, "USD", got["assetCode"], "get-by-id must preserve the retained assetCode")
		assert.Equal(t, v1CreateResp["routeId"], got["routeId"], "routeId is retained on the v2 read exactly as on v1")

		assertV2TransactionOmitsDeprecated(t, got, "v2 get-by-id", true)
	})

	t.Run("list", func(t *testing.T) {
		resp := getV2(t, v2App, v2TxListURL(infra.orgID, infra.ledgerID))
		page := decodeTxResponse(t, resp, nethttp.StatusOK)

		items, ok := page["items"].([]any)
		require.True(t, ok, "list envelope must carry an items array")
		require.NotEmpty(t, items, "list must return the seeded transaction")

		for i, raw := range items {
			item, ok := raw.(map[string]any)
			require.Truef(t, ok, "list item[%d] must decode as an object", i)

			assertV2TransactionOmitsDeprecated(t, item, "v2 list item", true)
		}
	})

	t.Run("patch", func(t *testing.T) {
		resp := patchV2(t, v2App, v2TxByIDURL(infra.orgID, infra.ledgerID, txID), `{"description":"v2 patched description"}`)
		got := decodeTxResponse(t, resp, nethttp.StatusOK)

		assert.Equal(t, "v2 patched description", got["description"], "PATCH must apply the description update")
		assert.Equal(t, txID.String(), got["id"], "PATCH must return the updated transaction")

		// The update core (command.UpdateTransaction + query.GetTransactionByID) does not project
		// operations, so the PATCH response carries none — requireOps is false; the top-level
		// field-absence guard still holds.
		assertV2TransactionOmitsDeprecated(t, got, "v2 patch", false)
	})
}

// TestIntegration_TransactionV2MirrorGetByID_CacheHit proves the /v2 get-by-id op wires the shared
// read core's cache flag onto the X-Cache-Hit response header. The flag reflects the write-behind
// store, which a plain DB read never repopulates, so both a cold and a second (warm) read report the
// header — present and carrying a boolean string — as appropriate for the store's state. In this
// synchronous harness (RABBITMQ_TRANSACTION_ASYNC=false, balance-sync drained) the write-behind slot
// is not standing, so the value is "false"; the contract proven is that the header is WIRED off the
// core's cacheHit return, not left unset.
func TestIntegration_TransactionV2MirrorGetByID_CacheHit(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	ctx := context.Background()

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	v1App := buildHumaTransactionApp(t, infra.handler, true)
	v2App := buildHumaV2MirrorApp(t, infra.handler)

	v1CreateResp := decodeTxResponse(t, postTransaction(t, v1App, v1JSONURL(infra.orgID, infra.ledgerID), equivalentV1Body, ""), nethttp.StatusCreated)
	txID := uuid.MustParse(v1CreateResp["id"].(string))
	drainBalanceSync(t, ctx, infra.handler.Command, infra.redisRepo, infra.orgID, infra.ledgerID)

	url := v2TxByIDURL(infra.orgID, infra.ledgerID, txID)

	valid := []string{"true", "false"}

	first := getV2(t, v2App, url)
	_ = decodeTxResponse(t, first, nethttp.StatusOK)
	firstHit := first.Header.Get("X-Cache-Hit")
	assert.Containsf(t, valid, firstHit, "cold get-by-id must carry a boolean X-Cache-Hit, got %q", firstHit)

	// A second (warm) read still carries the header off the same core return.
	second := getV2(t, v2App, url)
	_ = decodeTxResponse(t, second, nethttp.StatusOK)
	secondHit := second.Header.Get("X-Cache-Hit")
	assert.Containsf(t, valid, secondHit, "warm get-by-id must carry a boolean X-Cache-Hit, got %q", secondHit)
}
