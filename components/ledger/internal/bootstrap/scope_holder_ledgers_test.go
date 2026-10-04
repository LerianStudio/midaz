// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ledgerPartnerAuthz stands in for the Access Manager deciding for a partner scoped
// on one ledger: a question naming another ledger is denied, any other is allowed.
func ledgerPartnerAuthz(t *testing.T, allowedLedger string) (*httptest.Server, func() []string) {
	t.Helper()

	var (
		mu     sync.Mutex
		ledger []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("ledger partner authz: decode authorization body: %v", err)
		}

		asked, named := body.Attributes["ledgerId"]

		mu.Lock()
		if named {
			ledger = append(ledger, asked)
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if named && asked != allowedLedger {
			_, _ = w.Write([]byte(`{"authorized":false}`))

			return
		}

		_, _ = w.Write([]byte(`{"authorized":true}`))
	}))

	t.Cleanup(server.Close)

	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()

		return append([]string(nil), ledger...)
	}
}

// TestScopeResolvers_HolderByIDThroughTheRouter drives the holder detail routes,
// with the boot resolvers and the real guard, for a partner scoped on one ledger:
// a holder is visible when any ledger it owns a live account in is allowed.
func TestScopeResolvers_HolderByIDThroughTheRouter(t *testing.T) {
	unsetDocsGate(t)

	org, ownLedger, siblingLedger := uuid.New(), uuid.New(), uuid.New()
	inOwn, inSibling, inBoth, noAccounts := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	fake := &fakeScopeResolver{holders: map[uuid.UUID][]uuid.UUID{
		inOwn:     {ownLedger},
		inSibling: {siblingLedger},
		inBoth:    {siblingLedger, ownLedger},
	}}

	authz, askedLedgers := ledgerPartnerAuthz(t, ownLedger.String())

	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL}
	require.NoError(t, registerScopeResolvers(auth, fake, nil, scopeInstruments{}))
	require.NoError(t, wireAuthScope(auth))

	server := buildFullSurfaceServerWithAuth(t, auth)

	send := func(method string, holder uuid.UUID) int {
		t.Helper()

		req := httptest.NewRequest(method, "/v2/organizations/"+org.String()+"/holders/"+holder.String(), nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+scopeProbePartnerToken(t))

		resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	for _, method := range []string{fiber.MethodGet, fiber.MethodPatch, fiber.MethodDelete} {
		t.Run(method+" a holder with an account in the partner's ledger is allowed", func(t *testing.T) {
			before := len(askedLedgers())

			assert.NotEqual(t, fiber.StatusForbidden, send(method, inOwn))
			assert.Equal(t, []string{ownLedger.String()}, askedLedgers()[before:])
		})

		t.Run(method+" a holder only in a sibling ledger is refused", func(t *testing.T) {
			before := len(askedLedgers())

			assert.Equal(t, fiber.StatusForbidden, send(method, inSibling))
			assert.Equal(t, []string{siblingLedger.String()}, askedLedgers()[before:])
		})

		t.Run(method+" a holder in both ledgers is allowed once any of them is", func(t *testing.T) {
			before := len(askedLedgers())

			assert.NotEqual(t, fiber.StatusForbidden, send(method, inBoth))
			assert.Equal(t, []string{siblingLedger.String(), ownLedger.String()}, askedLedgers()[before:])
		})

		t.Run(method+" a holder without a live account is refused without asking a ledger", func(t *testing.T) {
			before := len(askedLedgers())

			assert.Equal(t, fiber.StatusForbidden, send(method, noAccounts))
			assert.Empty(t, askedLedgers()[before:])
		})
	}

	for _, call := range fake.holderCalls {
		assert.Equal(t, org, call.organizationID, "every holder lookup is confined to the organization of the path")
	}
}
