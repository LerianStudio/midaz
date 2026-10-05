// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	instrumentrepo "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// partnerDecider stands in for the Access Manager deciding for a partner holding
// scope: a named dimension it scopes must be one of its values, and a filtered
// dimension it scopes is answered with its values. A dimension it does not scope
// confines nothing.
type partnerDecider struct {
	mu       sync.Mutex
	scope    map[string][]string
	filtered []string
}

func newPartnerDecider(t *testing.T, scope map[string][]string) (*partnerDecider, *httptest.Server) {
	t.Helper()

	decider := &partnerDecider{scope: scope}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
			Filter     []string          `json:"filter"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("partner decider: decode authorization body: %v", err)
		}

		decider.mu.Lock()
		decider.filtered = body.Filter
		decider.mu.Unlock()

		allowed := make(map[string][]string)
		authorized := true

		for dimension, values := range scope {
			if named, ok := body.Attributes[dimension]; ok && named != "" {
				authorized = authorized && slices.Contains(values, named)

				continue
			}

			if slices.Contains(body.Filter, dimension) {
				allowed[dimension] = values
			}
		}

		answer := map[string]any{"authorized": authorized}
		if authorized && len(allowed) > 0 {
			answer["allowed"] = allowed
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(answer); err != nil {
			t.Errorf("partner decider: write response: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	return decider, server
}

func (d *partnerDecider) askedFilter() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.filtered...)
}

// mountPartnerListRoutes mounts the production v2 holder-accounts and instrument
// registrars behind a lib-auth client wired from the embedded manifest exactly as
// the boot wires it.
func mountPartnerListRoutes(t *testing.T, authzURL string, holderAccounts *HolderAccountsHandler, instruments *InstrumentHandler) *fiber.App {
	t.Helper()

	auth := &middleware.AuthClient{Address: authzURL, Enabled: true}
	wireManifestScope(t, auth)

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	v2 := app.Group("/v2")
	api := openapi.New(app, v2, openapi.Config{Title: "partner-lists", Version: "test", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(api)
	RegisterHolderAccountsV2RoutesToApp(v2, api, auth, holderAccounts, nil)
	RegisterInstrumentV2RoutesToApp(v2, api, auth, instruments, nil, nil)

	return app
}

func sendPartnerList(t *testing.T, app *fiber.App, target, token string) int {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodGet, target, nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode
}

// TestGetAccountsByHolder_PartnerConfinedByEveryAccountDimension drives the
// holder's account list through the real guard and the embedded manifest: the
// list asks for, and confines on, every dimension the account list does, plus the
// ledger, since a holder's accounts span the organization.
func TestGetAccountsByHolder_PartnerConfinedByEveryAccountDimension(t *testing.T) {
	org, holder := uuid.New(), uuid.New()
	ledgerA, portfolio1, segment1, account1 := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	target := "/v2/organizations/" + org.String() + "/holders/" + holder.String() + "/accounts"

	tests := []struct {
		name  string
		scope map[string][]string
		token string
		want  pkgHTTP.ScopeConfinement
	}{
		{
			name:  "a portfolio partner reads only the accounts of its portfolio",
			scope: map[string][]string{"ledgerId": {ledgerA.String()}, "portfolioId": {portfolio1.String()}},
			want:  pkgHTTP.ScopeConfinement{"ledgerId": {ledgerA}, "portfolioId": {portfolio1}},
		},
		{
			name:  "an account partner reads only its accounts",
			scope: map[string][]string{"ledgerId": {ledgerA.String()}, "accountId": {account1.String()}},
			want:  pkgHTTP.ScopeConfinement{"ledgerId": {ledgerA}, "accountId": {account1}},
		},
		{
			name:  "a segment partner reads only the accounts of its segment",
			scope: map[string][]string{"ledgerId": {ledgerA.String()}, "segmentId": {segment1.String()}},
			want:  pkgHTTP.ScopeConfinement{"ledgerId": {ledgerA}, "segmentId": {segment1}},
		},
		{
			name:  "a ledger partner reads the whole ledger",
			scope: map[string][]string{"ledgerId": {ledgerA.String()}},
			want:  pkgHTTP.ScopeConfinement{"ledgerId": {ledgerA}},
		},
		{
			name:  "an empty allowed list reads nothing",
			scope: map[string][]string{"ledgerId": {ledgerA.String()}, "portfolioId": {}},
			want:  pkgHTTP.ScopeConfinement{"ledgerId": {ledgerA}, "portfolioId": {}},
		},
		{
			name:  "a partner scoping no account dimension is not confined",
			scope: map[string][]string{},
			want:  nil,
		},
		{
			name:  "a caller bound to no partner is not confined",
			scope: map[string][]string{"ledgerId": {ledgerA.String()}},
			token: guardBearerToken(t),
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decider, server := newPartnerDecider(t, tt.scope)
			reader := &stubHolderAccountsReader{accounts: []*mmodel.Account{}}
			app := mountPartnerListRoutes(t, server.URL, &HolderAccountsHandler{Reader: reader}, &InstrumentHandler{})

			token := tt.token
			if token == "" {
				token = partnerListToken(t)

				defer func() {
					assert.ElementsMatch(t, []string{"ledgerId", "accountId", "portfolioId", "segmentId"}, decider.askedFilter(),
						"the partner question must ask for every dimension the holder's accounts are confined on")
				}()
			}

			require.Equal(t, fiber.StatusOK, sendPartnerList(t, app, target, token))
			assert.Equal(t, tt.want, reader.gotScope)
		})
	}
}

// TestGetAllInstruments_PartnerConfinedByHolder drives the instrument list through
// the real guard and the embedded manifest for a partner scoped on a holder: an
// instrument read by id is confined to the holder its path names, so the list is
// confined to the partner's holders too.
func TestGetAllInstruments_PartnerConfinedByHolder(t *testing.T) {
	org, holder := uuid.New(), uuid.New()

	ctrl := gomock.NewController(t)
	repo := instrumentrepo.NewMockRepository(ctrl)

	var gotScope pkgHTTP.ScopeConfinement

	repo.EXPECT().FindAll(gomock.Any(), org.String(), uuid.Nil, gomock.Any(), false).
		DoAndReturn(func(_, _, _ any, q pkgHTTP.QueryHeader, _ bool) ([]*mmodel.Instrument, error) {
			gotScope = q.Scope

			return []*mmodel.Instrument{}, nil
		})

	decider, server := newPartnerDecider(t, map[string][]string{"holderId": {holder.String()}})
	app := mountPartnerListRoutes(t, server.URL, &HolderAccountsHandler{Reader: &stubHolderAccountsReader{}},
		&InstrumentHandler{Service: &services.UseCase{InstrumentRepo: repo}})

	require.Equal(t, fiber.StatusOK, sendPartnerList(t, app, "/v2/organizations/"+org.String()+"/instruments", partnerListToken(t)))
	assert.Equal(t, pkgHTTP.ScopeConfinement{"holderId": {holder}}, gotScope)
	assert.ElementsMatch(t, []string{"ledgerId", "accountId", "holderId"}, decider.askedFilter())
}
