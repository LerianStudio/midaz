//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// fixedTenantPG hands every tenant the one test database, recording who asked.
type fixedTenantPG struct {
	db    dbresolver.DB
	asked []string
}

func (f *fixedTenantPG) tenantDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	f.asked = append(f.asked, tenantID)

	return f.db, nil
}

// TestScopeResolvers_MultiTenantReachTheTenantDatabase drives a partner request on a
// resolved route through the real router, with repositories that refuse to serve
// without a tenant connection on the context. The resolver takes the tenant from the
// validated credential and attaches its database, so the alias resolves; a credential
// naming no tenant is refused as unavailable, never served from a default database.
func TestScopeResolvers_MultiTenantReachTheTenantDatabase(t *testing.T) {
	unsetDocsGate(t)

	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	conn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)

	db, err := conn.Resolver(t.Context())
	require.NoError(t, err)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	aliceID := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "Alice", "@alice", "USD", nil)

	holderID := uuid.New()
	holderParams := pgtestutil.DefaultAccountParams()
	holderParams.Alias = "@held"
	holderParams.HolderID = &holderID
	pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, holderParams)

	portfolioID := pgtestutil.CreateTestPortfolio(t, container.DB, orgID, ledgerID)
	placedParams := pgtestutil.DefaultAccountParams()
	placedParams.Alias = "@placed"
	placedParams.PortfolioID = &portfolioID
	placedID := pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, placedParams)

	uc := &query.UseCase{AccountRepo: account.NewAccountPostgreSQLRepository(nil, true)}
	tenantPG := &fixedTenantPG{db: db}

	recorder, authz := newAllowRecorder(t)
	// M2M inversion off: the resolver still receives the validated principal.
	auth := &middleware.AuthClient{Enabled: true, Address: authz.URL, M2MInversionEnabled: false}

	require.NoError(t, registerScopeResolvers(auth, uc, &multiTenantScope{pg: map[string]tenantPGSource{constant.ModuleOnboarding: tenantPG}}))
	require.NoError(t, wireAuthScope(auth))

	server := buildFullSurfaceServerWithAuth(t, auth)

	aliasPath := "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/accounts/alias/@alice"
	holderPath := "/v2/organizations/" + orgID.String() + "/holders/" + holderID.String()

	sendTo := func(path string, claims jwt.MapClaims) int {
		t.Helper()

		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("mt-secret"))
		require.NoError(t, err)

		req := httptest.NewRequest(fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

		resp, err := server.app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	send := func(claims jwt.MapClaims) int { return sendTo(aliasPath, claims) }

	tenant := uuid.NewString()
	partner := jwt.MapClaims{"type": "application", "owner": "mt-org", "sub": "mt-org/mt-app", "partner": "mt-partner"}

	t.Run("the credential's tenant database resolves the alias", func(t *testing.T) {
		recorder.reset()

		claims := jwt.MapClaims{"tenantId": tenant}
		for k, v := range partner {
			claims[k] = v
		}

		send(claims)

		resolved := recorder.resolved()
		require.Len(t, resolved, 1)
		assert.Equal(t, aliceID.String(), resolved[0]["accountId"])
		canonical, err := tmcore.CanonicalTenantID(tenant)
		require.NoError(t, err)
		assert.Contains(t, tenantPG.asked, canonical, "the database of the credential's own tenant")
	})

	t.Run("a credential naming no tenant is refused as unavailable", func(t *testing.T) {
		recorder.reset()

		assert.Equal(t, fiber.StatusServiceUnavailable, send(partner))
		assert.Empty(t, recorder.resolved())
	})

	t.Run("the credential's tenant database resolves the ledgers of a holder", func(t *testing.T) {
		recorder.reset()

		claims := jwt.MapClaims{"tenantId": tenant}
		for k, v := range partner {
			claims[k] = v
		}

		sendTo(holderPath, claims)

		var ledgers []string

		for _, attrs := range recorder.snapshot() {
			if ledger, ok := attrs["ledgerId"]; ok {
				ledgers = append(ledgers, ledger)
			}
		}

		assert.Equal(t, []string{ledgerID.String()}, ledgers, "the holder's ledger, read from its tenant's accounts")
	})

	t.Run("the credential's tenant database resolves the portfolio of an account", func(t *testing.T) {
		recorder.reset()

		claims := jwt.MapClaims{"tenantId": tenant}
		for k, v := range partner {
			claims[k] = v
		}

		sendTo("/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+placedID.String(), claims)

		var portfolios []string

		for _, attrs := range recorder.snapshot() {
			if portfolio, ok := attrs["portfolioId"]; ok {
				portfolios = append(portfolios, portfolio)
			}
		}

		assert.Equal(t, []string{portfolioID.String()}, portfolios, "the account's portfolio, read from its tenant's accounts")
	})

	t.Run("a holder lookup for a credential naming no tenant is refused as unavailable", func(t *testing.T) {
		recorder.reset()

		assert.Equal(t, fiber.StatusServiceUnavailable, sendTo(holderPath, partner))
	})
}

// snapshot returns every question recorded so far.
func (r *allowRecorder) snapshot() []map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]map[string]string(nil), r.attributes...)
}
