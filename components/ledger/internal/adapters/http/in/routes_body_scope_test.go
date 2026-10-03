// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// bodyScopeAuthz stands in for the Access Manager on the routes that carry their
// organization and ledger in the request body. It records every question it is
// asked and allows exactly the (organization, ledger) pairs it was given; any other
// question is denied.
type bodyScopeAuthz struct {
	mu        sync.Mutex
	allowAll  bool
	allowed   map[[2]string]bool
	questions []map[string]string
}

func newBodyScopeAuthz(t *testing.T, allowed ...[2]string) (*bodyScopeAuthz, *httptest.Server) {
	t.Helper()

	authz := &bodyScopeAuthz{allowed: make(map[[2]string]bool, len(allowed))}
	for _, pair := range allowed {
		authz.allowed[pair] = true
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body scope authz: decode request body: %v", err)
		}

		authz.mu.Lock()
		authz.questions = append(authz.questions, body.Attributes)
		allow := authz.allowAll || authz.allowed[[2]string{body.Attributes["organizationId"], body.Attributes["ledgerId"]}]
		authz.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		answer := `{"authorized":false}`
		if allow {
			answer = `{"authorized":true}`
		}

		if _, err := w.Write([]byte(answer)); err != nil {
			t.Errorf("body scope authz: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return authz, server
}

// newTenantAuthz stands in for the Access Manager answering a tenant credential,
// which no partner scope narrows: every question is allowed.
func newTenantAuthz(t *testing.T) (*bodyScopeAuthz, *httptest.Server) {
	t.Helper()

	authz, server := newBodyScopeAuthz(t)
	authz.allowAll = true

	return authz, server
}

func (a *bodyScopeAuthz) asked() []map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]map[string]string(nil), a.questions...)
}

// bodyScopeToken signs a credential. A non-empty partner binds it to that partner,
// the way the identity service mints a partner application's token; an empty one
// is a tenant credential.
func bodyScopeToken(t *testing.T, partner string) string {
	t.Helper()

	claims := jwt.MapClaims{"type": "application", "owner": "scope-org", "sub": "scope-org/app"}
	if partner != "" {
		claims["partner"] = partner
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("scope-secret"))
	require.NoError(t, err)

	return signed
}

// bodyScopeGuardChain records what the chain behind the authorization guard saw.
// It stands in for the handler: it answers 204 and stops the chain, so a request it
// observed is one the guard let through, and one it did not observe never reached
// the handler.
type bodyScopeGuardChain struct {
	reached int
	scope   middleware.RequestScope
	scoped  bool
	body    []byte
}

// mountBodyScopeV2 mounts the production v2 transaction and instrument registrars on
// a /v2 group, behind a lib-auth client wired from the embedded manifest exactly as
// the boot wires it.
func mountBodyScopeV2(t *testing.T, authzURL string) (*fiber.App, *bodyScopeGuardChain) {
	t.Helper()

	auth := &middleware.AuthClient{Address: authzURL, Enabled: true}
	wireManifestScope(t, auth)

	chain := &bodyScopeGuardChain{}
	options := &pkgHTTP.ProtectedRouteOptions{PostAuthMiddlewares: []fiber.Handler{func(c fiber.Ctx) error {
		chain.reached++
		chain.scope, chain.scoped = middleware.ScopeFromContext(c.Context())
		chain.body = append([]byte(nil), c.Body()...)

		return c.SendStatus(fiber.StatusNoContent)
	}}}

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	group := app.Group("/v2")
	api := openapi.New(app, group, openapi.Config{Title: "body-scope", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(api)

	RegisterTransactionV2RoutesToApp(group, api, auth, &TransactionHandler{}, options)
	RegisterInstrumentV2RoutesToApp(group, api, auth, &InstrumentHandler{}, options, options)

	return app, chain
}

func postBodyScope(t *testing.T, app *fiber.App, path, token, body string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(raw)
}

// leg renders one transaction leg naming its organization and ledger.
func leg(alias, organizationID, ledgerID string) string {
	return `{"alias":"` + alias + `","organizationId":"` + organizationID + `","ledgerId":"` + ledgerID + `","amount":"1"}`
}

func directBody(debits, credits []string) string {
	return `{"asset":"BRL","amount":"2","debits":[` + strings.Join(debits, ",") + `],"credits":[` + strings.Join(credits, ",") + `]}`
}

func pairQuestion(organizationID, ledgerID string) map[string]string {
	return map[string]string{"organizationId": organizationID, "ledgerId": ledgerID}
}

// legQuestion is the question one leg asks: its organization and ledger, and the
// account its alias resolves to within that ledger.
func legQuestion(organizationID, ledgerID, alias string) map[string]string {
	return map[string]string{"organizationId": organizationID, "ledgerId": ledgerID, "accountId": legAccount(ledgerID, alias)}
}

// TestBodyScope_V2CreateAsksOncePerDistinctPair drives every v2 create action with
// legs spread over two ledgers of one organization, all inside the partner's scope.
// Each leg asks about its organization, its ledger and the account its alias
// resolves to within that ledger, and the guard lets the request through.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook.
func TestBodyScope_V2CreateAsksOncePerDistinctPair(t *testing.T) {
	org := uuid.NewString()
	ledgerA, ledgerB := uuid.NewString(), uuid.NewString()

	body := directBody(
		[]string{leg("@a", org, ledgerA), leg("@b", org, ledgerB)},
		[]string{leg("@c", org, ledgerA)},
	)

	for _, action := range []string{"direct", "hold", "block", "unblock"} {
		t.Run(action, func(t *testing.T) {
			authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerA}, [2]string{org, ledgerB})
			app, chain := mountBodyScopeV2(t, server.URL)

			status, raw := postBodyScope(t, app, "/v2/transactions/"+action, bodyScopeToken(t, "partner-a"), body)

			want := []map[string]string{legQuestion(org, ledgerA, "@a"), legQuestion(org, ledgerB, "@b"), legQuestion(org, ledgerA, "@c")}

			require.Equalf(t, fiber.StatusNoContent, status, "a body inside the scope must reach the handler: %s", raw)
			assert.Equal(t, append([]map[string]string{pairQuestion(org, ledgerA), pairQuestion(org, ledgerB)}, want...), authz.asked(),
				"the credential is first asked about each distinct pair, then each leg with its alias resolved within its own ledger")
			assert.Equal(t, 1, chain.reached)

			require.True(t, chain.scoped, "a partner request must carry its authorized scope to the handler")
			assert.Equal(t, "partner-a", chain.scope.Partner)
			assert.Equal(t, want, chain.scope.Sets)
			assert.Equal(t, map[string]string{"organizationId": org}, chain.scope.Attributes,
				"only the identifier every set shares is a request-wide attribute")
			assert.JSONEq(t, body, string(chain.body), "the handler must read the body untouched")
		})
	}
}

// TestBodyScope_V2CreateRefusesALegOutsideTheScope moves ONE leg into a ledger the
// partner does not hold. The request is refused with 403 and nothing behind the
// guard runs, whichever side the stray leg sits on.
func TestBodyScope_V2CreateRefusesALegOutsideTheScope(t *testing.T) {
	org := uuid.NewString()
	ledgerIn, ledgerOut := uuid.NewString(), uuid.NewString()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "stray debit",
			body: directBody([]string{leg("@a", org, ledgerIn), leg("@b", org, ledgerOut)}, []string{leg("@c", org, ledgerIn)}),
		},
		{
			name: "stray credit",
			body: directBody([]string{leg("@a", org, ledgerIn)}, []string{leg("@c", org, ledgerIn), leg("@d", org, ledgerOut)}),
		},
		{
			name: "stray organization",
			body: directBody([]string{leg("@a", org, ledgerIn)}, []string{leg("@c", uuid.NewString(), ledgerIn)}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerIn})
			app, chain := mountBodyScopeV2(t, server.URL)

			status, raw := postBodyScope(t, app, "/v2/transactions/direct", bodyScopeToken(t, "partner-a"), tc.body)

			assert.Equalf(t, fiber.StatusForbidden, status, "a leg outside the scope must refuse the request: %s", raw)
			assert.Zero(t, chain.reached, "nothing behind the guard may run on a refused request")

			asked := authz.asked()
			require.NotEmpty(t, asked)
			last := asked[len(asked)-1]
			assert.Falsef(t, last["organizationId"] == org && last["ledgerId"] == ledgerIn,
				"the refusal must come from the stray leg, asked last: %v", last)
		})
	}
}

// TestBodyScope_V2CreateRefusesAnUnreadableBody sends bodies the guard cannot read
// its dimensions from. Each is answered 400 naming the exact field, before any
// authorization call and without reaching the handler.
func TestBodyScope_V2CreateRefusesAnUnreadableBody(t *testing.T) {
	org, ledgerID := uuid.NewString(), uuid.NewString()

	tests := []struct {
		name  string
		path  string
		body  string
		field string
	}{
		{
			name:  "leg without a ledger",
			path:  "/v2/transactions/direct",
			body:  directBody([]string{leg("@a", org, ledgerID)}, []string{leg("@c", org, ledgerID), `{"alias":"@d","organizationId":"` + org + `"}`}),
			field: "credits[1].ledgerId",
		},
		{
			name:  "ledger that is not a string",
			path:  "/v2/transactions/hold",
			body:  directBody([]string{`{"alias":"@a","organizationId":"` + org + `","ledgerId":7}`}, []string{leg("@c", org, ledgerID)}),
			field: "debits[0].ledgerId",
		},
		{
			name:  "empty organization",
			path:  "/v2/transactions/direct",
			body:  directBody([]string{leg("@a", "", ledgerID)}, []string{leg("@c", org, ledgerID)}),
			field: "debits[0].organizationId",
		},
		{
			name:  "no debits",
			path:  "/v2/transactions/direct",
			body:  `{"asset":"BRL","amount":"1","credits":[` + leg("@c", org, ledgerID) + `]}`,
			field: "debits",
		},
		{
			name:  "batch item leg without an organization",
			path:  "/v2/transactions/batch",
			body:  `{"transactions":[{"action":"direct","order":1,"asset":"BRL","amount":"1","debits":[` + leg("@a", org, ledgerID) + `],"credits":[{"alias":"@c","ledgerId":"` + ledgerID + `"}]}]}`,
			field: "transactions[0].credits[0].organizationId",
		},
		{
			name:  "not JSON",
			path:  "/v2/transactions/direct",
			body:  `debits=1`,
			field: "debits[].organizationId",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerID})
			app, chain := mountBodyScopeV2(t, server.URL)

			status, raw := postBodyScope(t, app, tc.path, bodyScopeToken(t, "partner-a"), tc.body)

			assert.Equalf(t, fiber.StatusBadRequest, status, "an unreadable body must be refused as the caller's: %s", raw)
			assert.Containsf(t, raw, `\"`+tc.field+`\"`, "the refusal must name the field %s", tc.field)
			assert.Empty(t, authz.asked(), "no authorization call on a body the guard cannot read")
			assert.Zero(t, chain.reached, "nothing behind the guard may run on a refused request")
		})
	}
}

// TestBodyScope_V2BatchAsksAboutEveryItem spreads the legs of a two-item batch over
// three pairs. Inside the scope, every distinct pair is asked about once; with one
// leg of the SECOND item outside, the whole batch is refused.
func TestBodyScope_V2BatchAsksAboutEveryItem(t *testing.T) {
	org := uuid.NewString()
	ledgerA, ledgerB, ledgerC := uuid.NewString(), uuid.NewString(), uuid.NewString()

	item := func(order string, debit, credit string) string {
		return `{"action":"direct","order":` + order + `,"asset":"BRL","amount":"1","debits":[` + debit + `],"credits":[` + credit + `]}`
	}

	body := `{"transactions":[` +
		item("1", leg("@a", org, ledgerA), leg("@b", org, ledgerB)) + `,` +
		item("2", leg("@c", org, ledgerB), leg("@d", org, ledgerC)) +
		`]}`

	t.Run("every item inside the scope", func(t *testing.T) {
		authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerA}, [2]string{org, ledgerB}, [2]string{org, ledgerC})
		app, chain := mountBodyScopeV2(t, server.URL)

		status, raw := postBodyScope(t, app, "/v2/transactions/batch", bodyScopeToken(t, "partner-a"), body)

		require.Equalf(t, fiber.StatusNoContent, status, "a batch inside the scope must reach the handler: %s", raw)
		assert.ElementsMatch(t,
			[]map[string]string{
				pairQuestion(org, ledgerA), pairQuestion(org, ledgerB), pairQuestion(org, ledgerC),
				legQuestion(org, ledgerA, "@a"), legQuestion(org, ledgerB, "@b"), legQuestion(org, ledgerB, "@c"), legQuestion(org, ledgerC, "@d"),
			},
			authz.asked(), "each distinct pair, then each leg across every item with its alias resolved within its own ledger")
		assert.Equal(t, 1, chain.reached)
	})

	t.Run("one leg of the second item outside the scope", func(t *testing.T) {
		authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerA}, [2]string{org, ledgerB})
		app, chain := mountBodyScopeV2(t, server.URL)

		status, raw := postBodyScope(t, app, "/v2/transactions/batch", bodyScopeToken(t, "partner-a"), body)

		assert.Equalf(t, fiber.StatusForbidden, status, "a stray leg anywhere in the batch must refuse it: %s", raw)
		assert.Zero(t, chain.reached)
		assert.Contains(t, authz.asked(), pairQuestion(org, ledgerC), "the stray leg's ledger must have been asked about")
	})
}

// TestBodyScope_InstrumentCreateAsksAboutTheBodyLedger covers the one route whose
// path names the organization while the ledger rides in the body: the question
// carries both, plus the holder the path names and the account the body names, and
// a ledger outside the scope is refused before the handler.
func TestBodyScope_InstrumentCreateAsksAboutTheBodyLedger(t *testing.T) {
	org, holder := uuid.NewString(), uuid.NewString()
	ledgerIn, ledgerOut := uuid.NewString(), uuid.NewString()

	path := "/v2/organizations/" + org + "/holders/" + holder + "/instruments"
	account := uuid.NewString()
	instrument := func(ledgerID string) string {
		return `{"ledgerId":"` + ledgerID + `","accountId":"` + account + `"}`
	}

	t.Run("ledger inside the scope", func(t *testing.T) {
		authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerIn})
		app, chain := mountBodyScopeV2(t, server.URL)

		status, raw := postBodyScope(t, app, path, bodyScopeToken(t, "partner-a"), instrument(ledgerIn))

		require.Equalf(t, fiber.StatusNoContent, status, "%s", raw)
		assert.Equal(t, []map[string]string{{"organizationId": org, "holderId": holder, "ledgerId": ledgerIn, "accountId": account}}, authz.asked())
		assert.Equal(t, 1, chain.reached)
	})

	t.Run("ledger outside the scope", func(t *testing.T) {
		_, server := newBodyScopeAuthz(t, [2]string{org, ledgerIn})
		app, chain := mountBodyScopeV2(t, server.URL)

		status, raw := postBodyScope(t, app, path, bodyScopeToken(t, "partner-a"), instrument(ledgerOut))

		assert.Equalf(t, fiber.StatusForbidden, status, "%s", raw)
		assert.Zero(t, chain.reached)
	})

	t.Run("ledger missing", func(t *testing.T) {
		authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerIn})
		app, chain := mountBodyScopeV2(t, server.URL)

		status, raw := postBodyScope(t, app, path, bodyScopeToken(t, "partner-a"), `{"accountId":"`+uuid.NewString()+`"}`)

		assert.Equalf(t, fiber.StatusBadRequest, status, "%s", raw)
		assert.Contains(t, raw, `\"ledgerId\"`)
		assert.Empty(t, authz.asked())
		assert.Zero(t, chain.reached)
	})
	t.Run("account missing", func(t *testing.T) {
		authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerIn})
		app, chain := mountBodyScopeV2(t, server.URL)

		status, raw := postBodyScope(t, app, path, bodyScopeToken(t, "partner-a"), `{"ledgerId":"`+ledgerIn+`"}`)

		assert.Equalf(t, fiber.StatusBadRequest, status, "%s", raw)
		assert.Contains(t, raw, `\"accountId\"`)
		assert.Empty(t, authz.asked())
		assert.Zero(t, chain.reached)
	})
}

// TestBodyScope_TenantCredentialIsUnchanged drives the same routes with a tenant
// credential — one bound to no partner. It is decided exactly as before body
// dimensions existed: one authorization call carrying only the dimensions the path
// derives, a body the guard never parses — so one a partner would be refused for
// still reaches the handler — read untouched, and no partner scope published.
func TestBodyScope_TenantCredentialIsUnchanged(t *testing.T) {
	org, ledgerID, holder := uuid.NewString(), uuid.NewString(), uuid.NewString()

	tests := []struct {
		name string
		path string
		body string
		want map[string]string
	}{
		{
			name: "direct",
			path: "/v2/transactions/direct",
			body: directBody([]string{leg("@a", org, ledgerID)}, []string{leg("@c", org, ledgerID)}),
		},
		{
			name: "direct with a leg naming no ledger",
			path: "/v2/transactions/direct",
			body: directBody([]string{leg("@a", org, ledgerID)}, []string{`{"alias":"@c","organizationId":"` + org + `"}`}),
		},
		{
			name: "hold with a body that is not JSON",
			path: "/v2/transactions/hold",
			body: `debits=1`,
		},
		{
			name: "batch",
			path: "/v2/transactions/batch",
			body: `{"transactions":[{"action":"direct","order":1,"asset":"BRL","amount":"1","debits":[` +
				leg("@a", org, ledgerID) + `],"credits":[` + leg("@c", org, ledgerID) + `]}]}`,
		},
		{
			name: "instrument create",
			path: "/v2/organizations/" + org + "/holders/" + holder + "/instruments",
			body: `{"ledgerId":"` + ledgerID + `","accountId":"` + uuid.NewString() + `"}`,
			want: map[string]string{"organizationId": org, "holderId": holder},
		},
		{
			name: "instrument create naming no ledger",
			path: "/v2/organizations/" + org + "/holders/" + holder + "/instruments",
			body: `{"accountId":"` + uuid.NewString() + `"}`,
			want: map[string]string{"organizationId": org, "holderId": holder},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authz, server := newTenantAuthz(t)
			app, chain := mountBodyScopeV2(t, server.URL)

			status, raw := postBodyScope(t, app, tc.path, bodyScopeToken(t, ""), tc.body)

			require.Equalf(t, fiber.StatusNoContent, status, "a tenant credential must reach the handler: %s", raw)
			assert.Equal(t, 1, chain.reached)
			assert.Equal(t, []map[string]string{tc.want}, authz.asked(),
				"a tenant credential must make one call carrying only the dimensions the path derives")
			assert.False(t, chain.scoped, "a tenant credential must not be published as a partner scope")
			assert.Equal(t, tc.body, string(chain.body), "the handler must read the body untouched")
		})
	}
}

// TestBodyScope_V2SameAliasInTwoLedgersAsksTwoAccounts names one alias on two legs of
// different ledgers: each resolves within its own leg's ledger, so the guard asks
// about two accounts, not one.
func TestBodyScope_V2SameAliasInTwoLedgersAsksTwoAccounts(t *testing.T) {
	org := uuid.NewString()
	ledgerA, ledgerB := uuid.NewString(), uuid.NewString()

	authz, server := newBodyScopeAuthz(t, [2]string{org, ledgerA}, [2]string{org, ledgerB})
	app, chain := mountBodyScopeV2(t, server.URL)

	status, raw := postBodyScope(t, app, "/v2/transactions/direct", bodyScopeToken(t, "partner-a"),
		directBody([]string{leg("@shared", org, ledgerA)}, []string{leg("@shared", org, ledgerB)}))

	require.Equalf(t, fiber.StatusNoContent, status, "%s", raw)
	assert.Equal(t, []map[string]string{
		pairQuestion(org, ledgerA), pairQuestion(org, ledgerB),
		legQuestion(org, ledgerA, "@shared"), legQuestion(org, ledgerB, "@shared"),
	}, authz.asked())
	assert.Equal(t, 1, chain.reached)
}
