// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScopeResolvers_InstrumentByIDThroughTheRouter drives the instrument detail
// routes, with the boot resolvers and the real guard: an instrument is reached by
// a partner of the ledger it names itself, the rule the instruments list is
// confined by.
func TestScopeResolvers_InstrumentByIDThroughTheRouter(t *testing.T) {
	unsetDocsGate(t)

	org, holder := uuid.New(), uuid.New()
	ownLedger, siblingLedger := uuid.New(), uuid.New()
	inOwn, inSibling, noLedger := uuid.New(), uuid.New(), uuid.New()

	instruments := &fakeInstrumentLedgers{ledgers: map[uuid.UUID]string{
		inOwn:     ownLedger.String(),
		inSibling: siblingLedger.String(),
	}}

	authz, server := newScopedPartnerAuthz(t)

	auth := &middleware.AuthClient{Enabled: true, Address: server.URL}
	require.NoError(t, registerScopeResolvers(auth, &fakeScopeResolver{}, nil, scopeInstruments{reader: instruments}))
	require.NoError(t, wireAuthScope(auth))

	app := buildFullSurfaceServerWithAuth(t, auth)

	paths := map[string]func(instrument uuid.UUID) string{
		fiber.MethodGet:    func(i uuid.UUID) string { return "/instruments/" + i.String() },
		fiber.MethodPatch:  func(i uuid.UUID) string { return "/instruments/" + i.String() },
		fiber.MethodDelete: func(i uuid.UUID) string { return "/instruments/" + i.String() },
		"DELETE related party": func(i uuid.UUID) string {
			return "/instruments/" + i.String() + "/related-parties/" + uuid.NewString()
		},
	}

	send := func(row string, instrument uuid.UUID) int {
		t.Helper()

		method := strings.Fields(row)[0]

		var body *strings.Reader
		if method == fiber.MethodPatch {
			body = strings.NewReader(`{"metadata":{"k":"v"}}`)
		} else {
			body = strings.NewReader("")
		}

		req := httptest.NewRequest(method, "/v2/organizations/"+org.String()+"/holders/"+holder.String()+paths[row](instrument), body)
		if method == fiber.MethodPatch {
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		}

		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+scopeProbePartnerToken(t))

		resp, err := app.app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	ledgerPartner := map[string][]string{"ledgerId": {ownLedger.String()}}

	for row := range paths {
		t.Run(row+": a ledger partner reaches an instrument of its ledger", func(t *testing.T) {
			authz.reset(ledgerPartner)

			assert.NotEqual(t, fiber.StatusForbidden, send(row, inOwn))
			assert.Equal(t, []string{ownLedger.String()}, authz.asked("ledgerId"))
		})

		t.Run(row+": a ledger partner is refused an instrument of a sibling ledger", func(t *testing.T) {
			authz.reset(ledgerPartner)

			assert.Equal(t, fiber.StatusForbidden, send(row, inSibling))
			assert.Equal(t, []string{siblingLedger.String()}, authz.asked("ledgerId"))
		})

		t.Run(row+": a ledger partner is refused an instrument naming no ledger", func(t *testing.T) {
			authz.reset(ledgerPartner)

			assert.Equal(t, fiber.StatusForbidden, send(row, noLedger))
			assert.Empty(t, authz.asked("ledgerId"))
		})

		t.Run(row+": a holder partner reaches its holder's instrument naming no ledger", func(t *testing.T) {
			authz.reset(map[string][]string{"holderId": {holder.String()}})

			assert.NotEqual(t, fiber.StatusForbidden, send(row, noLedger))
			assert.NotEmpty(t, authz.asked("holderId"), "the decision came from the authorization service")
		})
	}

	for _, call := range instruments.calls {
		assert.Equal(t, org.String(), call.organizationID)
		assert.Equal(t, holder, call.holderID, "every instrument lookup is confined to the path's holder")
	}
}
