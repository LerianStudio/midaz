// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
)

// fakeScopeResolver answers from fixed tables and records the scope it was asked in.
type fakeScopeResolver struct {
	aliases      map[string]uuid.UUID
	transactions map[uuid.UUID][]uuid.UUID
	balances     map[uuid.UUID]uuid.UUID
	err          error

	aliasCalls  [][]string
	scopes      [][2]uuid.UUID
	transaction []uuid.UUID
}

func (f *fakeScopeResolver) AccountIDsByAlias(_ context.Context, organizationID, ledgerID uuid.UUID, aliases []string) (*query.AliasResolution, error) {
	f.aliasCalls = append(f.aliasCalls, aliases)
	f.scopes = append(f.scopes, [2]uuid.UUID{organizationID, ledgerID})

	if f.err != nil {
		return nil, f.err
	}

	out := &query.AliasResolution{AccountIDs: map[string]uuid.UUID{}}

	for _, alias := range aliases {
		if id, ok := f.aliases[alias]; ok {
			out.AccountIDs[alias] = id
		} else {
			out.NotFound = append(out.NotFound, alias)
		}
	}

	return out, nil
}

func (f *fakeScopeResolver) AccountIDsOfTransaction(_ context.Context, organizationID, ledgerID, transactionID uuid.UUID) ([]uuid.UUID, bool, error) {
	f.scopes = append(f.scopes, [2]uuid.UUID{organizationID, ledgerID})
	f.transaction = append(f.transaction, transactionID)

	if f.err != nil {
		return nil, false, f.err
	}

	ids, ok := f.transactions[transactionID]

	return ids, ok, nil
}

func (f *fakeScopeResolver) AccountIDOfBalance(_ context.Context, organizationID, ledgerID, balanceID uuid.UUID) (uuid.UUID, bool, error) {
	f.scopes = append(f.scopes, [2]uuid.UUID{organizationID, ledgerID})

	if f.err != nil {
		return uuid.Nil, false, f.err
	}

	id, ok := f.balances[balanceID]

	return id, ok, nil
}

func resolveInput(org, ledger uuid.UUID, values ...string) middleware.ResolveInput {
	return middleware.ResolveInput{
		Product:   "midaz",
		Dimension: "accountId",
		Values:    values,
		Known: map[string][]string{
			"organizationId": {org.String()},
			"ledgerId":       {ledger.String()},
		},
	}
}

func TestScopeResolvers_AccountByAlias(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	alice, bob := uuid.New(), uuid.New()

	t.Run("each alias answers its account, keyed by the value the request carried", func(t *testing.T) {
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice, "@bob": bob}}

		out, err := scopeResolvers{fake}.accountByAlias(context.Background(), resolveInput(org, ledger, "@alice", "@bob#savings", "0#@alice#default"))
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{
			"@alice":           {alice.String()},
			"@bob#savings":     {bob.String()},
			"0#@alice#default": {alice.String()},
		}, out)
		assert.Equal(t, [][]string{{"@alice", "@bob"}}, fake.aliasCalls, "one batched lookup of the bare aliases")
		assert.Equal(t, [][2]uuid.UUID{{org, ledger}}, fake.scopes, "confined to the organization and ledger of the path")
	})

	t.Run("an alias no live account holds is left out, so the request is refused naming it", func(t *testing.T) {
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice}}

		out, err := scopeResolvers{fake}.accountByAlias(context.Background(), resolveInput(org, ledger, "@alice", "@ghost"))
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{"@alice": {alice.String()}}, out)
	})

	t.Run("a lookup failure is an error", func(t *testing.T) {
		fake := &fakeScopeResolver{err: query.ScopeAliasAmbiguousError{Alias: "@twin"}}

		_, err := scopeResolvers{fake}.accountByAlias(context.Background(), resolveInput(org, ledger, "@twin"))
		require.Error(t, err)
	})

	t.Run("a request naming no single organization and ledger cannot be confined", func(t *testing.T) {
		fake := &fakeScopeResolver{}

		for name, known := range map[string]map[string][]string{
			"none":            nil,
			"no ledger":       {"organizationId": {org.String()}},
			"two ledgers":     {"organizationId": {org.String()}, "ledgerId": {ledger.String(), uuid.NewString()}},
			"no organization": {"ledgerId": {ledger.String()}},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := scopeResolvers{fake}.accountByAlias(context.Background(), middleware.ResolveInput{Values: []string{"@alice"}, Known: known})
				require.ErrorIs(t, err, errScopeResolverUnconfined)
			})
		}

		assert.Empty(t, fake.aliasCalls, "no lookup runs without a confinement")
	})

	t.Run("a path coordinate that is not a uuid resolves nothing", func(t *testing.T) {
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice}}

		out, err := scopeResolvers{fake}.accountByAlias(context.Background(), middleware.ResolveInput{
			Values: []string{"@alice"},
			Known:  map[string][]string{"organizationId": {"not-a-uuid"}, "ledgerId": {ledger.String()}},
		})
		require.NoError(t, err)
		assert.Empty(t, out)
		assert.Empty(t, fake.aliasCalls)
	})
}

func TestScopeResolvers_ExternalAccount(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	external := uuid.New()

	fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@external/BRL": external}}

	out, err := scopeResolvers{fake}.externalAccount(context.Background(), resolveInput(org, ledger, "BRL", "USD"))
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"BRL": {external.String()}}, out)
	assert.Equal(t, [][]string{{"@external/BRL", "@external/USD"}}, fake.aliasCalls)
}

func TestScopeResolvers_TransactionAccounts(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	pending, settled, unknown := uuid.New(), uuid.New(), uuid.New()
	a, b, c := uuid.New(), uuid.New(), uuid.New()

	t.Run("every transaction answers the accounts of its legs; an unknown one is left out", func(t *testing.T) {
		fake := &fakeScopeResolver{transactions: map[uuid.UUID][]uuid.UUID{pending: {a, b}, settled: {a, b, c}}}

		out, err := scopeResolvers{fake}.transactionAccounts(context.Background(),
			resolveInput(org, ledger, pending.String(), settled.String(), unknown.String(), "not-a-uuid"))
		require.NoError(t, err)
		assert.Equal(t, map[string][]string{
			pending.String(): {a.String(), b.String()},
			settled.String(): {a.String(), b.String(), c.String()},
		}, out)
		assert.Equal(t, []uuid.UUID{pending, settled, unknown}, fake.transaction, "a value that is not a uuid is never looked up")
	})

	t.Run("a lookup failure is an error", func(t *testing.T) {
		fake := &fakeScopeResolver{err: query.ErrScopeTransactionAccountsUnavailable}

		_, err := scopeResolvers{fake}.transactionAccounts(context.Background(), resolveInput(org, ledger, pending.String()))
		require.ErrorIs(t, err, query.ErrScopeTransactionAccountsUnavailable)
	})
}

func TestScopeResolvers_BalanceAccount(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	balance, unknown, account := uuid.New(), uuid.New(), uuid.New()

	fake := &fakeScopeResolver{balances: map[uuid.UUID]uuid.UUID{balance: account}}

	out, err := scopeResolvers{fake}.balanceAccount(context.Background(), resolveInput(org, ledger, balance.String(), unknown.String()))
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{balance.String(): {account.String()}}, out)

	fake.err = errors.New("replica down")

	_, err = scopeResolvers{fake}.balanceAccount(context.Background(), resolveInput(org, ledger, balance.String()))
	require.Error(t, err)
}

func TestScopeResolvers_RegisteredUnderTheManifestNames(t *testing.T) {
	auth := &middleware.AuthClient{Enabled: true}

	require.NoError(t, registerScopeResolvers(auth, &fakeScopeResolver{}))
	require.Error(t, registerScopeResolvers(auth, &fakeScopeResolver{}), "a second registration under the same names is refused")

	for _, name := range []string{resolverAccountByAlias, resolverExternalAccount, resolverTransactionAccounts, resolverBalanceAccount} {
		assert.Error(t, auth.RegisterScopeResolver(name, func(context.Context, middleware.ResolveInput) (map[string][]string, error) { return nil, nil }),
			"%s must already be registered", name)
	}
}

// probeResolvedAccount is the value every probe resolver answers: the probe value of
// the accountId dimension, so a resolved request asks about the same account a path
// naming it would.
func probeResolvedAccount(t *testing.T) string {
	t.Helper()

	for i, dim := range manifestScopeDimensions(t) {
		if dim.Name == "accountId" {
			return scopeProbeValue(i)
		}
	}

	require.Fail(t, "the manifest must declare the accountId dimension")

	return ""
}

// wireProbeAuthScope registers a probe resolver under every name the manifest uses,
// each answering probeResolvedAccount for any value, and wires the scope as boot does.
func wireProbeAuthScope(t *testing.T, auth *middleware.AuthClient) {
	t.Helper()

	account := probeResolvedAccount(t)

	for _, name := range []string{resolverAccountByAlias, resolverExternalAccount, resolverTransactionAccounts, resolverBalanceAccount} {
		require.NoError(t, auth.RegisterScopeResolver(name, func(_ context.Context, in middleware.ResolveInput) (map[string][]string, error) {
			out := make(map[string][]string, len(in.Values))
			for _, value := range in.Values {
				out[value] = []string{account}
			}

			return out, nil
		}))
	}

	require.NoError(t, wireAuthScope(auth), "the boot scope wiring must accept the embedded manifest")
}

func TestScopeResolvers_TheManifestNeedsEveryRegisteredName(t *testing.T) {
	require.Error(t, wireAuthScope(&middleware.AuthClient{Enabled: true}),
		"a manifest that names resolvers must be refused until they are registered")

	auth := &middleware.AuthClient{Enabled: true}
	require.NoError(t, registerScopeResolvers(auth, &fakeScopeResolver{}))
	require.NoError(t, wireAuthScope(auth), "the boot registration must cover every resolver the manifest names")
}

// TestScopeResolvers_ThroughTheRouter drives the boot resolvers, backed by a fixed
// ScopeResolver, through the real router with a partner credential.
func TestScopeResolvers_ThroughTheRouter(t *testing.T) {
	unsetDocsGate(t)

	org, ledger := uuid.New(), uuid.New()
	alice, source, destination := uuid.New(), uuid.New(), uuid.New()
	pending := uuid.New()

	fake := &fakeScopeResolver{
		aliases:      map[string]uuid.UUID{"@alice": alice},
		transactions: map[uuid.UUID][]uuid.UUID{pending: {source, destination}},
	}

	recorder, authz := newAllowRecorder(t)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	require.NoError(t, registerScopeResolvers(auth, fake))
	require.NoError(t, wireAuthScope(auth))

	server := buildFullSurfaceServerWithAuth(t, auth)
	base := "/v1/organizations/" + org.String() + "/ledgers/" + ledger.String()

	send := func(method, target, body string) int {
		t.Helper()

		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}

		req := httptest.NewRequest(method, target, reader)
		if body != "" {
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		}

		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+scopeProbePartnerToken(t))

		resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		raw, _ := io.ReadAll(resp.Body)
		t.Logf("%s %s -> %d %s", method, target, resp.StatusCode, raw)

		return resp.StatusCode
	}

	t.Run("a known alias is asked as its account", func(t *testing.T) {
		recorder.reset()

		send(fiber.MethodGet, base+"/accounts/alias/@alice", "")
		require.Len(t, recorder.attributes, 1)
		assert.Equal(t, alice.String(), recorder.attributes[0]["accountId"])
		assert.Equal(t, org.String(), recorder.attributes[0]["organizationId"])
	})

	t.Run("an unknown alias is refused with 422 before any authorization call", func(t *testing.T) {
		recorder.reset()

		assert.Equal(t, fiber.StatusUnprocessableEntity, send(fiber.MethodGet, base+"/accounts/alias/@ghost", ""))
		assert.Empty(t, recorder.attributes)
	})

	t.Run("every leg of a transaction is asked", func(t *testing.T) {
		recorder.reset()

		send(fiber.MethodPost, base+"/transactions/"+pending.String()+"/commit", "")

		asked := make([]string, 0, len(recorder.attributes))
		for _, attrs := range recorder.attributes {
			asked = append(asked, attrs["accountId"])
		}

		assert.ElementsMatch(t, []string{source.String(), destination.String()}, asked)
	})

	t.Run("every alias of a v1 create body is asked", func(t *testing.T) {
		recorder.reset()

		send(fiber.MethodPost, base+"/transactions/json",
			`{"send":{"asset":"BRL","value":"1","source":{"from":[{"accountAlias":"@alice#default"}]},"distribute":{"to":[{"accountAlias":"@alice"}]}}}`)
		require.Len(t, recorder.attributes, 1, "two legs naming one account ask once")
		assert.Equal(t, alice.String(), recorder.attributes[0]["accountId"])
	})

	t.Run("a failed lookup is refused with 503", func(t *testing.T) {
		recorder.reset()
		fake.err = query.ScopeAliasAmbiguousError{Alias: "@alice"}

		defer func() { fake.err = nil }()

		assert.Equal(t, fiber.StatusServiceUnavailable, send(fiber.MethodGet, base+"/accounts/alias/@alice", ""))
		assert.Empty(t, recorder.attributes)
	})
}

// allowRecorder stands in for the Access Manager: it allows every question and
// records the attributes of each.
type allowRecorder struct {
	mu         sync.Mutex
	attributes []map[string]string
}

func (r *allowRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attributes = nil
}

func newAllowRecorder(t *testing.T) (*allowRecorder, *httptest.Server) {
	t.Helper()

	recorder := &allowRecorder{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("allow recorder: decode authorization body: %v", err)
		}

		recorder.mu.Lock()
		recorder.attributes = append(recorder.attributes, body.Attributes)
		recorder.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if _, err := w.Write([]byte(`{"authorized":true}`)); err != nil {
			t.Errorf("allow recorder: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return recorder, server
}
