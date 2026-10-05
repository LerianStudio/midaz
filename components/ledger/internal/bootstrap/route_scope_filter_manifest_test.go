// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wantListFilters pins every list route that confines its result to the values
// the authorization service allows a partner, and the dimensions it filters on.
var wantListFilters = map[string][]string{
	"GET\t/v1/organizations/:organization_id/ledgers/:ledger_id/accounts":                    {"accountId", "portfolioId", "segmentId"},
	"GET\t/v2/organizations/:organization_id/ledgers/:ledger_id/accounts":                    {"accountId", "portfolioId", "segmentId"},
	"HEAD\t/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/metrics/count":     {"accountId", "portfolioId", "segmentId"},
	"HEAD\t/v2/organizations/:organization_id/ledgers/:ledger_id/accounts/metrics/count":     {"accountId", "portfolioId", "segmentId"},
	"GET\t/v1/organizations/:organization_id/ledgers/:ledger_id/balances":                    {"accountId"},
	"GET\t/v2/organizations/:organization_id/ledgers/:ledger_id/balances":                    {"accountId"},
	"GET\t/v1/organizations/:organization_id/ledgers/:ledger_id/transactions":                {"accountId"},
	"GET\t/v2/organizations/:organization_id/ledgers/:ledger_id/transactions":                {"accountId"},
	"HEAD\t/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count": {"accountId"},
	"HEAD\t/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count": {"accountId"},
	"GET\t/v2/organizations/:organization_id/ledgers/:ledger_id/fee-debts":                   {"accountId"},
	"GET\t/v2/organizations/:organization_id/holders":                                        {"ledgerId", "accountId"},
	"GET\t/v2/organizations/:organization_id/holders/:holder_id/accounts":                    {"ledgerId", "accountId", "portfolioId", "segmentId"},
	"GET\t/v2/organizations/:organization_id/instruments":                                    {"ledgerId", "accountId", "holderId"},
}

// TestManifestScope_ListFiltersArePinned ties the manifest's filter declarations to
// the lists whose handlers confine their result: a list declared without a handler
// that reads the allowed values would serve every row to a partner.
func TestManifestScope_ListFiltersArePinned(t *testing.T) {
	got := make(map[string][]string)

	for _, r := range manifestScopeRoutes(t) {
		if len(r.Filter) > 0 {
			got[scopeRouteKey(r)] = r.Filter
		}
	}

	assert.Equal(t, wantListFilters, got)
}

// TestManifestScope_HolderAccountsFilterOnEveryAccountDimension ties the holder's
// account list to the account list: both list accounts, so the holder's list
// filters on every dimension the account list does, plus the ledger its path
// leaves out.
func TestManifestScope_HolderAccountsFilterOnEveryAccountDimension(t *testing.T) {
	filters := make(map[string][]string)

	for _, r := range manifestScopeRoutes(t) {
		filters[scopeRouteKey(r)] = r.Filter
	}

	accounts := filters["GET\t/v2/organizations/:organization_id/ledgers/:ledger_id/accounts"]
	require.NotEmpty(t, accounts, "the account list must filter")

	assert.ElementsMatch(t, append([]string{"ledgerId"}, accounts...),
		filters["GET\t/v2/organizations/:organization_id/holders/:holder_id/accounts"])
}

// TestManifestScope_ListFiltersReachTheAuthorizationService drives every filtering
// list through the real router with a partner credential and asserts the question
// asks the authorization service for the allowed values of each filtered dimension.
func TestManifestScope_ListFiltersReachTheAuthorizationService(t *testing.T) {
	unsetDocsGate(t)

	var asked []string

	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Filter []string `json:"filter"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("filter recorder: decode authorization body: %v", err)
		}

		asked = body.Filter

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorized":false}`))
	}))
	t.Cleanup(authz.Close)

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	wireProbeAuthScope(t, auth)

	server := buildFullSurfaceServerWithAuth(t, auth)
	groups := groupRouteRows(collectRouteRows(t, server.app))

	dims := manifestScopeDimensions(t)

	values := make(map[string]string, len(dims))
	for i, dim := range dims {
		values[dim.Param] = scopeProbeValue(i)
	}

	driven := 0

	for _, group := range groups {
		want, filters := wantListFilters[group.key]
		if !filters {
			continue
		}

		driven++

		t.Run(group.display(), func(t *testing.T) {
			asked = nil

			req := httptest.NewRequest(group.rows[0].method, scopedRouteURL(group.rows[0].path, values), nil)
			req.Header.Set(fiber.HeaderAuthorization, "Bearer "+scopeProbePartnerToken(t))

			resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			assert.ElementsMatch(t, want, asked, "the partner question must ask for the allowed values of every filtered dimension")
		})
	}

	assert.Equal(t, len(wantListFilters), driven, "every filtering list must be mounted")
}
