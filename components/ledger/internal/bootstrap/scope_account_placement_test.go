// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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

// scopedPartnerAuthz stands in for the Access Manager deciding for a partner whose
// scope lines name, per dimension, the values it may reach. A question is allowed
// when every dimension the partner is scoped on is named with an allowed value, or
// is still pending resolution; a dimension the partner is not scoped on never denies.
type scopedPartnerAuthz struct {
	mu        sync.Mutex
	scope     map[string][]string
	questions []map[string]string
}

func newScopedPartnerAuthz(t *testing.T) (*scopedPartnerAuthz, *httptest.Server) {
	t.Helper()

	authz := &scopedPartnerAuthz{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
			Pending    []string          `json:"pending"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("scoped partner authz: decode authorization body: %v", err)
		}

		authz.mu.Lock()
		authz.questions = append(authz.questions, body.Attributes)
		allowed := true

		for dimension, values := range authz.scope {
			value, named := body.Attributes[dimension]

			switch {
			case named && !slices.Contains(values, value):
				allowed = false
			case !named && !slices.Contains(body.Pending, dimension):
				allowed = false
			}
		}
		authz.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if allowed {
			_, _ = w.Write([]byte(`{"authorized":true}`))

			return
		}

		_, _ = w.Write([]byte(`{"authorized":false}`))
	}))

	t.Cleanup(server.Close)

	return authz, server
}

func (a *scopedPartnerAuthz) reset(scope map[string][]string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.scope, a.questions = scope, nil
}

// asked returns every value the questions named for a dimension.
func (a *scopedPartnerAuthz) asked(dimension string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	var out []string

	for _, question := range a.questions {
		if value, named := question[dimension]; named && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}

	return out
}

// TestScopeResolvers_AccountByIDThroughTheRouter drives the account detail routes,
// with the boot resolvers and the real guard, for partners scoped on each dimension
// that reaches an account.
func TestScopeResolvers_AccountByIDThroughTheRouter(t *testing.T) {
	unsetDocsGate(t)

	org, ledger := uuid.New(), uuid.New()
	portfolio1, portfolio2, segment := uuid.New(), uuid.New(), uuid.New()
	inPortfolio1, inPortfolio2, bare, segmentOnly := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	fake := &fakeScopeResolver{placements: map[uuid.UUID]query.AccountPlacement{
		inPortfolio1: {PortfolioID: &portfolio1},
		inPortfolio2: {PortfolioID: &portfolio2},
		bare:         {},
		segmentOnly:  {SegmentID: &segment},
	}}

	authz, server := newScopedPartnerAuthz(t)

	auth := &middleware.AuthClient{Enabled: true, Address: server.URL}
	require.NoError(t, registerScopeResolvers(auth, fake, nil, scopeInstruments{}))
	require.NoError(t, wireAuthScope(auth))

	app := buildFullSurfaceServerWithAuth(t, auth)

	send := func(method, version string, account uuid.UUID, body string) int {
		t.Helper()

		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}

		target := "/" + version + "/organizations/" + org.String() + "/ledgers/" + ledger.String() + "/accounts/" + account.String()

		req := httptest.NewRequest(method, target, reader)
		if body != "" {
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		}

		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+scopeProbePartnerToken(t))

		resp, err := app.app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	portfolioPartner := map[string][]string{"portfolioId": {portfolio1.String()}}

	for _, version := range []string{"v1", "v2"} {
		for _, method := range []string{fiber.MethodGet, fiber.MethodPatch, fiber.MethodDelete} {
			body := ""
			if method == fiber.MethodPatch {
				body = `{"name":"renamed"}`
			}

			row := version + " " + method

			t.Run(row+": a portfolio partner reaches an account of its portfolio", func(t *testing.T) {
				authz.reset(portfolioPartner)

				assert.NotEqual(t, fiber.StatusForbidden, send(method, version, inPortfolio1, body))
				assert.NotEmpty(t, authz.asked("organizationId"), "the decision came from the authorization service")
				assert.Equal(t, []string{portfolio1.String()}, authz.asked("portfolioId"))
			})

			t.Run(row+": a portfolio partner is refused an account of a sibling portfolio", func(t *testing.T) {
				authz.reset(portfolioPartner)

				assert.Equal(t, fiber.StatusForbidden, send(method, version, inPortfolio2, body))
				assert.Equal(t, []string{portfolio2.String()}, authz.asked("portfolioId"))
			})

			t.Run(row+": a portfolio partner is refused an account in no portfolio", func(t *testing.T) {
				authz.reset(portfolioPartner)

				assert.Equal(t, fiber.StatusForbidden, send(method, version, bare, body))
				assert.Empty(t, authz.asked("portfolioId"))
			})

			t.Run(row+": a ledger partner reaches an account in no portfolio", func(t *testing.T) {
				authz.reset(map[string][]string{"ledgerId": {ledger.String()}})

				assert.NotEqual(t, fiber.StatusForbidden, send(method, version, bare, body))
				assert.NotEmpty(t, authz.asked("organizationId"), "the decision came from the authorization service")
			})

			t.Run(row+": an account partner reaches its own account in no portfolio", func(t *testing.T) {
				authz.reset(map[string][]string{"accountId": {bare.String()}})

				assert.NotEqual(t, fiber.StatusForbidden, send(method, version, bare, body))
				assert.NotEmpty(t, authz.asked("organizationId"), "the decision came from the authorization service")
			})

			t.Run(row+": a segment partner reaches an account of its segment in no portfolio", func(t *testing.T) {
				authz.reset(map[string][]string{"segmentId": {segment.String()}})

				assert.NotEqual(t, fiber.StatusForbidden, send(method, version, segmentOnly, body))
				assert.NotEmpty(t, authz.asked("organizationId"), "the decision came from the authorization service")
				assert.Equal(t, []string{segment.String()}, authz.asked("segmentId"))
			})
		}

		move := `{"portfolioId":"` + portfolio2.String() + `"}`

		t.Run(version+" PATCH: moving an account needs both portfolios allowed", func(t *testing.T) {
			authz.reset(map[string][]string{"portfolioId": {portfolio1.String(), portfolio2.String()}})

			assert.NotEqual(t, fiber.StatusForbidden, send(fiber.MethodPatch, version, inPortfolio1, move))
			assert.NotEmpty(t, authz.asked("organizationId"), "the decision came from the authorization service")
			assert.ElementsMatch(t, []string{portfolio1.String(), portfolio2.String()}, authz.asked("portfolioId"))
		})

		t.Run(version+" PATCH: pulling another portfolio's account into the allowed one is refused", func(t *testing.T) {
			authz.reset(map[string][]string{"portfolioId": {portfolio2.String()}})

			assert.Equal(t, fiber.StatusForbidden, send(fiber.MethodPatch, version, inPortfolio1, move))
		})

		t.Run(version+" PATCH: moving an account out of the allowed portfolio is refused", func(t *testing.T) {
			authz.reset(portfolioPartner)

			assert.Equal(t, fiber.StatusForbidden, send(fiber.MethodPatch, version, inPortfolio1, move))
		})
	}

	t.Run("v2 close: a portfolio partner closes an account of its portfolio and not a sibling's", func(t *testing.T) {
		close := func(account uuid.UUID) int {
			req := httptest.NewRequest(fiber.MethodPost, "/v2/organizations/"+org.String()+"/ledgers/"+ledger.String()+"/accounts/"+account.String()+"/close", nil)
			req.Header.Set(fiber.HeaderAuthorization, "Bearer "+scopeProbePartnerToken(t))

			resp, err := app.app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			return resp.StatusCode
		}

		authz.reset(portfolioPartner)
		assert.NotEqual(t, fiber.StatusForbidden, close(inPortfolio1))
		assert.NotEmpty(t, authz.asked("organizationId"), "the decision came from the authorization service")

		authz.reset(portfolioPartner)
		assert.Equal(t, fiber.StatusForbidden, close(inPortfolio2))
	})

	for _, call := range fake.placeCalls {
		assert.Equal(t, [2]uuid.UUID{org, ledger}, [2]uuid.UUID{call.organizationID, call.ledgerID}, "every placement lookup is confined to the ledger of the path")
	}
}
