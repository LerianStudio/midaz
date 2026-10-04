// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package scoperesolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// accountAuthz allows a question only when it names the allowed account, or
// leaves the account pending resolution, and records the accounts it was asked.
type accountAuthz struct {
	mu      sync.Mutex
	allowed string
	asked   []string
}

func (a *accountAuthz) serve(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
			Pending    []string          `json:"pending"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("account authz: decode authorization body: %v", err)
		}

		a.mu.Lock()
		account, named := body.Attributes["accountId"]
		if named {
			a.asked = append(a.asked, account)
		}

		allowed := (named && account == a.allowed) || (!named && slices.Contains(body.Pending, "accountId"))
		a.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"authorized":%t}`, allowed)
	}))

	t.Cleanup(server.Close)

	return server
}

// TestValidationResolvers_MultiTenantReadTheTenantDatabase drives a partner read of
// a validation by id through the real guard, with the real repository over a
// connection that refuses to serve without a tenant database on the context. The
// resolver takes the tenant from the validated credential and attaches its
// database through the tenant pool, so the validation's account is read from
// where it was stored; a credential naming no tenant is refused as unavailable.
func TestValidationResolvers_MultiTenantReadTheTenantDatabase(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	accountA1, accountA2 := uuid.New(), uuid.New()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	seed := func(account uuid.UUID) uuid.UUID {
		t.Helper()

		id := uuid.New()
		_, err := db.Exec(`
			INSERT INTO transaction_validations
				(id, request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, decision, matched_rule_ids, evaluated_rule_ids,
				 processing_time_ms, reason, created_at)
			VALUES ($1,$2,'CARD',10,'USD',$3,$4,'ALLOW','{}','{}',1,'allowed',$3)`,
			id, uuid.New(), at, fmt.Sprintf(`{"accountId":%q,"type":"CHECKING"}`, account.String()))
		require.NoError(t, err)

		return id
	}

	ofA1, ofA2 := seed(accountA1), seed(accountA2)

	tenant := uuid.NewString()
	canonical := uuid.MustParse(tenant)
	canonicalID := fmt.Sprintf("%x", canonical[:])

	var (
		poolMu sync.Mutex
		pooled []string
	)

	pool := func(_ context.Context, tenantID string) (dbresolver.DB, error) {
		poolMu.Lock()
		pooled = append(pooled, tenantID)
		poolMu.Unlock()

		if tenantID != canonicalID {
			return nil, errors.New("tenant not provisioned")
		}

		return dbresolver.New(dbresolver.WithPrimaryDBs(db)), nil
	}

	// A connection with no static pool, in strict multi-tenant mode: it serves
	// only the tenant database a request context carries.
	conn := &pgdb.PostgresConnectionAdapter{}
	conn.SetMultiTenantEnabled(true)

	authz := &accountAuthz{allowed: accountA1.String()}
	server := authz.serve(t)

	authClient := &authMiddleware.AuthClient{Enabled: true, Address: server.URL}
	require.NoError(t, Register(authClient, postgres.NewTransactionValidationRepositoryWithConnection(conn),
		seamtenant.NewResolverWithPool(pool, true)))
	require.NoError(t, declaration.WireScope(authClient, tracerembed.TracerManifest))

	guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName}, authClient)

	app := fiber.New()
	app.Get("/v1/validations/:validation_id", guard.With("validations", "get", false), func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	read := func(id uuid.UUID, claims jwt.MapClaims) int {
		t.Helper()

		partner := jwt.MapClaims{"type": "application", "owner": "mt-org", "sub": "mt-org/mt-app", "partner": "mt-partner"}
		for k, v := range claims {
			partner[k] = v
		}

		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, partner).SignedString([]byte("mt-secret"))
		require.NoError(t, err)

		req := httptest.NewRequest(fiber.MethodGet, "/v1/validations/"+id.String(), nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	reset := func() {
		authz.mu.Lock()
		authz.asked = nil
		authz.mu.Unlock()

		poolMu.Lock()
		pooled = nil
		poolMu.Unlock()
	}

	inTenant := jwt.MapClaims{"tenantId": tenant}

	t.Run("a validation of the partner's account is read from the tenant database", func(t *testing.T) {
		reset()

		assert.Equal(t, fiber.StatusOK, read(ofA1, inTenant))
		assert.Equal(t, []string{accountA1.String()}, authz.asked)
		assert.Contains(t, pooled, canonicalID, "the database of the credential's own tenant")
	})

	t.Run("a validation of another account is refused", func(t *testing.T) {
		reset()

		assert.Equal(t, fiber.StatusForbidden, read(ofA2, inTenant))
		assert.Equal(t, []string{accountA2.String()}, authz.asked)
	})

	t.Run("an unknown validation is refused like one out of scope", func(t *testing.T) {
		reset()

		assert.Equal(t, fiber.StatusForbidden, read(uuid.New(), inTenant))
		assert.Empty(t, authz.asked)
	})

	t.Run("a credential naming another tenant reaches no database", func(t *testing.T) {
		reset()

		assert.Equal(t, fiber.StatusServiceUnavailable, read(ofA1, jwt.MapClaims{"tenantId": uuid.NewString()}))
		assert.Empty(t, authz.asked)
	})

	t.Run("a credential naming no tenant is refused as unavailable", func(t *testing.T) {
		reset()

		assert.Equal(t, fiber.StatusServiceUnavailable, read(ofA1, nil))
		assert.Empty(t, authz.asked)
		assert.Empty(t, pooled, "never served from a default database")
	})
}
