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
	holders      map[uuid.UUID][]uuid.UUID
	placements   map[uuid.UUID]query.AccountPlacement
	err          error

	aliasCalls  [][]string
	scopes      [][2]uuid.UUID
	transaction []uuid.UUID
	holderCalls []holderCall
	placeCalls  []placementCall
}

type placementCall struct {
	organizationID, ledgerID uuid.UUID
	accountIDs               []uuid.UUID
}

func (f *fakeScopeResolver) PlacementOfAccounts(_ context.Context, organizationID, ledgerID uuid.UUID, accountIDs []uuid.UUID) (map[uuid.UUID]query.AccountPlacement, error) {
	f.placeCalls = append(f.placeCalls, placementCall{organizationID: organizationID, ledgerID: ledgerID, accountIDs: accountIDs})

	if f.err != nil {
		return nil, f.err
	}

	out := map[uuid.UUID]query.AccountPlacement{}

	for _, id := range accountIDs {
		if placement, ok := f.placements[id]; ok {
			out[id] = placement
		}
	}

	return out, nil
}

type holderCall struct {
	organizationID uuid.UUID
	holderIDs      []uuid.UUID
}

func (f *fakeScopeResolver) LedgerIDsOfHolders(_ context.Context, organizationID uuid.UUID, holderIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	f.holderCalls = append(f.holderCalls, holderCall{organizationID: organizationID, holderIDs: holderIDs})

	if f.err != nil {
		return nil, f.err
	}

	out := map[uuid.UUID][]uuid.UUID{}

	for _, id := range holderIDs {
		if ledgers, ok := f.holders[id]; ok {
			out[id] = ledgers
		}
	}

	return out, nil
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
	items := make([]middleware.ResolveItem, 0, len(values))
	for _, value := range values {
		items = append(items, middleware.ResolveItem{Value: value})
	}

	return middleware.ResolveInput{
		Product:   "midaz",
		Dimension: "accountId",
		Items:     items,
		Known: map[string][]string{
			"organizationId": {org.String()},
			"ledgerId":       {ledger.String()},
		},
	}
}

func TestScopeResolvers_AccountByAlias(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	alice, bob := uuid.New(), uuid.New()

	t.Run("each alias answers its account, one entry per item in order", func(t *testing.T) {
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice, "@bob": bob}}

		out, err := scopeResolvers{resolver: fake}.accountByAlias(context.Background(), resolveInput(org, ledger, "@alice", "@bob#savings", "0#@alice#default"))
		require.NoError(t, err)
		assert.Equal(t, [][]string{{alice.String()}, {bob.String()}, {alice.String()}}, out)
		assert.Equal(t, [][]string{{"@alice", "@bob"}}, fake.aliasCalls, "one batched lookup of the bare aliases")
		assert.Equal(t, [][2]uuid.UUID{{org, ledger}}, fake.scopes, "confined to the organization and ledger of the path")
	})

	t.Run("an alias no live account holds maps to nothing, so the request is refused naming it", func(t *testing.T) {
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice}}

		out, err := scopeResolvers{resolver: fake}.accountByAlias(context.Background(), resolveInput(org, ledger, "@alice", "@ghost"))
		require.NoError(t, err)
		assert.Equal(t, [][]string{{alice.String()}, nil}, out)
	})

	t.Run("a body leg is looked up in the organization and ledger its own element names", func(t *testing.T) {
		otherOrg, otherLedger := uuid.New(), uuid.New()
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice}}

		out, err := scopeResolvers{resolver: fake}.accountByAlias(context.Background(), middleware.ResolveInput{Items: []middleware.ResolveItem{
			{Value: "@alice", Siblings: map[string]string{"organizationId": org.String(), "ledgerId": ledger.String()}},
			{Value: "@alice", Siblings: map[string]string{"organizationId": otherOrg.String(), "ledgerId": otherLedger.String()}},
		}})
		require.NoError(t, err)
		assert.Equal(t, [][]string{{alice.String()}, {alice.String()}}, out)
		assert.Equal(t, [][2]uuid.UUID{{org, ledger}, {otherOrg, otherLedger}}, fake.scopes, "one lookup per ledger the legs name")
	})

	t.Run("a lookup failure is an error", func(t *testing.T) {
		fake := &fakeScopeResolver{err: query.ScopeAliasAmbiguousError{Alias: "@twin"}}

		_, err := scopeResolvers{resolver: fake}.accountByAlias(context.Background(), resolveInput(org, ledger, "@twin"))
		require.Error(t, err)
	})

	t.Run("an item naming no single organization and ledger cannot be confined", func(t *testing.T) {
		fake := &fakeScopeResolver{}

		for name, known := range map[string]map[string][]string{
			"none":            nil,
			"no ledger":       {"organizationId": {org.String()}},
			"two ledgers":     {"organizationId": {org.String()}, "ledgerId": {ledger.String(), uuid.NewString()}},
			"no organization": {"ledgerId": {ledger.String()}},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := scopeResolvers{resolver: fake}.accountByAlias(context.Background(), middleware.ResolveInput{
					Items: []middleware.ResolveItem{{Value: "@alice"}}, Known: known,
				})
				require.ErrorIs(t, err, errScopeResolverUnconfined)
			})
		}

		assert.Empty(t, fake.aliasCalls, "no lookup runs without a confinement")
	})

	t.Run("a coordinate that is not a uuid resolves nothing", func(t *testing.T) {
		fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@alice": alice}}

		out, err := scopeResolvers{resolver: fake}.accountByAlias(context.Background(), middleware.ResolveInput{
			Items: []middleware.ResolveItem{{Value: "@alice", Siblings: map[string]string{"organizationId": "not-a-uuid", "ledgerId": ledger.String()}}},
		})
		require.NoError(t, err)
		assert.Equal(t, [][]string{nil}, out)
		assert.Empty(t, fake.aliasCalls)
	})
}

func TestScopeResolvers_ExternalAccount(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	external := uuid.New()

	fake := &fakeScopeResolver{aliases: map[string]uuid.UUID{"@external/BRL": external}}

	out, err := scopeResolvers{resolver: fake}.externalAccount(context.Background(), resolveInput(org, ledger, "BRL", "USD"))
	require.NoError(t, err)
	assert.Equal(t, [][]string{{external.String()}, nil}, out)
	assert.Equal(t, [][]string{{"@external/BRL", "@external/USD"}}, fake.aliasCalls)
}

func TestScopeResolvers_TransactionAccounts(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	pending, settled, unknown := uuid.New(), uuid.New(), uuid.New()
	a, b, c := uuid.New(), uuid.New(), uuid.New()

	t.Run("every transaction answers the accounts of its legs; an unknown one maps to nothing", func(t *testing.T) {
		fake := &fakeScopeResolver{transactions: map[uuid.UUID][]uuid.UUID{pending: {a, b}, settled: {a, b, c}}}

		out, err := scopeResolvers{resolver: fake}.transactionAccounts(context.Background(),
			resolveInput(org, ledger, pending.String(), settled.String(), unknown.String(), "not-a-uuid"))
		require.NoError(t, err)
		assert.Equal(t, [][]string{{a.String(), b.String()}, {a.String(), b.String(), c.String()}, nil, nil}, out)
		assert.Equal(t, []uuid.UUID{pending, settled, unknown}, fake.transaction, "a value that is not a uuid is never looked up")
	})

	t.Run("a lookup failure is an error", func(t *testing.T) {
		fake := &fakeScopeResolver{err: query.ErrScopeTransactionAccountsUnavailable}

		_, err := scopeResolvers{resolver: fake}.transactionAccounts(context.Background(), resolveInput(org, ledger, pending.String()))
		require.ErrorIs(t, err, query.ErrScopeTransactionAccountsUnavailable)
	})
}

func TestScopeResolvers_BalanceAccount(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	balance, unknown, account := uuid.New(), uuid.New(), uuid.New()

	fake := &fakeScopeResolver{balances: map[uuid.UUID]uuid.UUID{balance: account}}

	out, err := scopeResolvers{resolver: fake}.balanceAccount(context.Background(), resolveInput(org, ledger, balance.String(), unknown.String()))
	require.NoError(t, err)
	assert.Equal(t, [][]string{{account.String()}, nil}, out)

	fake.err = errors.New("replica down")

	_, err = scopeResolvers{resolver: fake}.balanceAccount(context.Background(), resolveInput(org, ledger, balance.String()))
	require.Error(t, err)
}

func TestScopeResolvers_HolderLedgers(t *testing.T) {
	org := uuid.New()
	both, one, none := uuid.New(), uuid.New(), uuid.New()
	ledger1, ledger2 := uuid.New(), uuid.New()

	holderInput := func(known map[string][]string, values ...string) middleware.ResolveInput {
		items := make([]middleware.ResolveItem, 0, len(values))
		for _, value := range values {
			items = append(items, middleware.ResolveItem{Value: value})
		}

		return middleware.ResolveInput{Product: "midaz", Dimension: "ledgerId", Items: items, Known: known}
	}

	inOrg := map[string][]string{"organizationId": {org.String()}}

	t.Run("each holder answers its ledgers, every holder in one read confined to the organization", func(t *testing.T) {
		fake := &fakeScopeResolver{holders: map[uuid.UUID][]uuid.UUID{both: {ledger1, ledger2}, one: {ledger2}}}

		out, err := scopeResolvers{resolver: fake}.holderLedgers(context.Background(), holderInput(inOrg, both.String(), none.String(), one.String(), "not-a-uuid"))
		require.NoError(t, err)
		assert.Equal(t, [][]string{{ledger1.String(), ledger2.String()}, nil, {ledger2.String()}, nil}, out,
			"a holder without a live account and a value that is not a uuid name nothing")
		assert.Equal(t, []holderCall{{organizationID: org, holderIDs: []uuid.UUID{both, none, one}}}, fake.holderCalls)
	})

	t.Run("a request naming no single organization is not resolved", func(t *testing.T) {
		fake := &fakeScopeResolver{}

		_, err := scopeResolvers{resolver: fake}.holderLedgers(context.Background(), holderInput(nil, both.String()))
		require.ErrorIs(t, err, errScopeResolverNoOrganization)

		_, err = scopeResolvers{resolver: fake}.holderLedgers(context.Background(),
			holderInput(map[string][]string{"organizationId": {org.String(), uuid.NewString()}}, both.String()))
		require.ErrorIs(t, err, errScopeResolverNoOrganization)
		assert.Empty(t, fake.holderCalls)
	})

	t.Run("an organization that is not a uuid names nothing and reads nothing", func(t *testing.T) {
		fake := &fakeScopeResolver{}

		out, err := scopeResolvers{resolver: fake}.holderLedgers(context.Background(),
			holderInput(map[string][]string{"organizationId": {"org"}}, both.String()))
		require.NoError(t, err)
		assert.Equal(t, [][]string{nil}, out)
		assert.Empty(t, fake.holderCalls)
	})

	t.Run("a failed read is an error", func(t *testing.T) {
		boom := errors.New("replica down")
		fake := &fakeScopeResolver{err: boom}

		_, err := scopeResolvers{resolver: fake}.holderLedgers(context.Background(), holderInput(inOrg, both.String()))
		require.ErrorIs(t, err, boom)
	})
}

func TestScopeResolvers_AccountPlacement(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	placed, bare, unknown := uuid.New(), uuid.New(), uuid.New()
	portfolio, segment := uuid.New(), uuid.New()

	fake := func() *fakeScopeResolver {
		return &fakeScopeResolver{placements: map[uuid.UUID]query.AccountPlacement{
			placed: {PortfolioID: &portfolio, SegmentID: &segment},
			bare:   {},
		}}
	}

	t.Run("each account answers its portfolio, every account in one read confined to the ledger", func(t *testing.T) {
		f := fake()

		out, err := scopeResolvers{resolver: f}.accountPortfolio(context.Background(), resolveInput(org, ledger, placed.String(), bare.String(), unknown.String(), "not-a-uuid"))
		require.NoError(t, err)
		assert.Equal(t, [][]string{{portfolio.String()}, nil, nil, nil}, out,
			"an account in no portfolio, an unknown account and a value that is not a uuid name nothing")
		assert.Equal(t, []placementCall{{organizationID: org, ledgerID: ledger, accountIDs: []uuid.UUID{placed, bare, unknown}}}, f.placeCalls)
	})

	t.Run("each account answers its segment", func(t *testing.T) {
		out, err := scopeResolvers{resolver: fake()}.accountSegment(context.Background(), resolveInput(org, ledger, placed.String(), bare.String()))
		require.NoError(t, err)
		assert.Equal(t, [][]string{{segment.String()}, nil}, out)
	})

	t.Run("a request naming no single ledger is not resolved", func(t *testing.T) {
		f := fake()
		in := resolveInput(org, ledger, placed.String())
		in.Known["ledgerId"] = nil

		_, err := scopeResolvers{resolver: f}.accountPortfolio(context.Background(), in)
		require.ErrorIs(t, err, errScopeResolverUnconfined)
		assert.Empty(t, f.placeCalls)
	})

	t.Run("a failed read is an error", func(t *testing.T) {
		boom := errors.New("replica down")
		f := fake()
		f.err = boom

		_, err := scopeResolvers{resolver: f}.accountSegment(context.Background(), resolveInput(org, ledger, placed.String()))
		require.ErrorIs(t, err, boom)
	})
}

func TestScopeResolvers_RegisteredUnderTheManifestNames(t *testing.T) {
	auth := &middleware.AuthClient{Enabled: true}

	require.NoError(t, registerScopeResolvers(auth, &fakeScopeResolver{}, nil))
	require.Error(t, registerScopeResolvers(auth, &fakeScopeResolver{}, nil), "a second registration under the same names is refused")

	for _, name := range []string{resolverAccountByAlias, resolverExternalAccount, resolverTransactionAccounts, resolverBalanceAccount, resolverHolderLedgers, resolverAccountPortfolio, resolverAccountSegment} {
		assert.Error(t, auth.RegisterScopeResolver(name, func(context.Context, middleware.ResolveInput) ([][]string, error) { return nil, nil }),
			"%s must already be registered", name)
	}
}

// wireProbeAuthScope registers a probe resolver under every name the manifest uses,
// each answering the probe value of the dimension it resolves for any value, and
// wires the scope as boot does.
func wireProbeAuthScope(t *testing.T, auth *middleware.AuthClient) {
	t.Helper()

	probes := make(map[string]string)
	for i, dim := range manifestScopeDimensions(t) {
		probes[dim.Name] = scopeProbeValue(i)
	}

	for _, name := range []string{resolverAccountByAlias, resolverExternalAccount, resolverTransactionAccounts, resolverBalanceAccount, resolverHolderLedgers, resolverAccountPortfolio, resolverAccountSegment} {
		require.NoError(t, auth.RegisterScopeResolver(name, func(_ context.Context, in middleware.ResolveInput) ([][]string, error) {
			out := make([][]string, len(in.Items))
			for i := range in.Items {
				out[i] = []string{probes[in.Dimension]}
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
	require.NoError(t, registerScopeResolvers(auth, &fakeScopeResolver{}, nil))
	require.NoError(t, wireAuthScope(auth), "the boot registration must cover every resolver the manifest names")
}

// TestScopeResolvers_ThroughTheRouter drives the boot resolvers, backed by a fixed
// ScopeResolver, through the real router with a partner credential.
func TestScopeResolvers_ThroughTheRouter(t *testing.T) {
	unsetDocsGate(t)

	org, ledger := uuid.New(), uuid.New()
	alice, source, destination := uuid.New(), uuid.New(), uuid.New()
	pending := uuid.New()

	bob, carol := uuid.New(), uuid.New()

	fake := &fakeScopeResolver{
		aliases:      map[string]uuid.UUID{"@alice": alice, "@bob": bob, "@carol": carol},
		transactions: map[uuid.UUID][]uuid.UUID{pending: {source, destination}},
	}

	recorder, authz := newAllowRecorder(t)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	require.NoError(t, registerScopeResolvers(auth, fake, nil))
	require.NoError(t, wireAuthScope(auth))

	server := buildFullSurfaceServerWithAuth(t, auth)
	base := "/v1/organizations/" + org.String() + "/ledgers/" + ledger.String()

	var lastBody string

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
		lastBody = string(raw)
		t.Logf("%s %s -> %d %s", method, target, resp.StatusCode, raw)

		return resp.StatusCode
	}

	t.Run("a known alias is asked as its account, after the credential is asked without it", func(t *testing.T) {
		recorder.reset()

		send(fiber.MethodGet, base+"/accounts/alias/@alice", "")
		require.Len(t, recorder.attributes, 2)
		assert.Equal(t, map[string]string{"organizationId": org.String(), "ledgerId": ledger.String()}, recorder.attributes[0])
		assert.Equal(t, alice.String(), recorder.attributes[1]["accountId"])
		assert.Equal(t, org.String(), recorder.attributes[1]["organizationId"])
	})

	t.Run("an unknown alias is refused with 403 and never asked as an account", func(t *testing.T) {
		recorder.reset()

		assert.Equal(t, fiber.StatusForbidden, send(fiber.MethodGet, base+"/accounts/alias/@ghost", ""))
		assert.Contains(t, lastBody, `path parameter \"alias\" is outside this credential's scope or does not exist`)
		assert.Contains(t, lastBody, `"code":"0043"`)
		assert.Empty(t, recorder.resolved())
	})

	t.Run("every leg of a transaction is asked", func(t *testing.T) {
		recorder.reset()

		send(fiber.MethodPost, base+"/transactions/"+pending.String()+"/commit", "")

		asked := make([]string, 0, 2)
		for _, attrs := range recorder.resolved() {
			asked = append(asked, attrs["accountId"])
		}

		assert.ElementsMatch(t, []string{source.String(), destination.String()}, asked)
	})

	t.Run("every alias of a v1 create body is asked", func(t *testing.T) {
		recorder.reset()

		send(fiber.MethodPost, base+"/transactions/json",
			`{"send":{"asset":"BRL","value":"1","source":{"from":[{"accountAlias":"@alice#default"}]},"distribute":{"to":[{"accountAlias":"@alice"}]}}}`)

		resolved := recorder.resolved()
		require.Len(t, resolved, 1, "two legs naming one account ask once")
		assert.Equal(t, alice.String(), resolved[0]["accountId"])
	})

	t.Run("every target alias and the maintenance credit account of a billing package is asked", func(t *testing.T) {
		recorder.reset()

		v2 := "/v2/organizations/" + org.String() + "/ledgers/" + ledger.String()
		send(fiber.MethodPost, v2+"/billing-packages",
			`{"accountTarget":{"aliases":["@alice","@bob"]},"maintenanceCreditAccount":"@carol"}`)

		asked := make([]string, 0, 3)
		for _, attrs := range recorder.resolved() {
			asked = append(asked, attrs["accountId"])
		}

		assert.ElementsMatch(t, []string{alice.String(), bob.String(), carol.String()}, asked)
	})

	t.Run("a failed lookup is refused with 503", func(t *testing.T) {
		recorder.reset()
		fake.err = query.ScopeAliasAmbiguousError{Alias: "@alice"}

		defer func() { fake.err = nil }()

		assert.Equal(t, fiber.StatusServiceUnavailable, send(fiber.MethodGet, base+"/accounts/alias/@alice", ""))
		assert.Empty(t, recorder.resolved())
	})
}

// resolved returns the questions that carried a resolved account.
func (r *allowRecorder) resolved() []map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []map[string]string

	for _, attrs := range r.attributes {
		if _, ok := attrs["accountId"]; ok {
			out = append(out, attrs)
		}
	}

	return out
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
