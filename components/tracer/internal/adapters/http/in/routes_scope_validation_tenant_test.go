// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/scoperesolver"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// tenantMarker is the context key a recordingTenant attaches its tenant under.
type tenantMarker struct{}

// recordingTenant stands in for the tenant database attachment: it records the
// tenant it is asked for and attaches it to the context, refusing an empty one.
type recordingTenant struct {
	mu     sync.Mutex
	active bool
	asked  []string
}

func (r *recordingTenant) Active() bool { return r.active }

func (r *recordingTenant) Resolve(ctx context.Context, tenantID string) (context.Context, error) {
	r.mu.Lock()
	r.asked = append(r.asked, tenantID)
	r.mu.Unlock()

	if tenantID == "" {
		return ctx, errors.New("tenant required")
	}

	return context.WithValue(ctx, tenantMarker{}, tenantID), nil
}

// tenantValidations answers a validation in the tenant it was stored in, and in
// the default database a context with no tenant attached falls back on; any
// other tenant has no database.
type tenantValidations struct {
	tenant string
	stored storedValidations
}

func (t tenantValidations) GetByID(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
	if tenant, attached := ctx.Value(tenantMarker{}).(string); attached && tenant != t.tenant {
		return nil, errors.New("no database attached for this tenant")
	}

	return t.stored.GetByID(ctx, id)
}

// TestTracerScope_ValidationDetailReadsTheCredentialTenant pins that a validation
// read by id is looked up in the database of the tenant the validated credential
// names, and never without one when the deployment is multi-tenant.
func TestTracerScope_ValidationDetailReadsTheCredentialTenant(t *testing.T) {
	account, own := uuid.New(), uuid.New()
	tenantID := "8f14e45f-ceea-467a-9575-6f2b6e0d1c3a"
	canonical := "8f14e45fceea467a95756f2b6e0d1c3a"

	validations := tenantValidations{tenant: canonical, stored: storedValidations{
		own: {ID: own, Account: model.AccountContext{ID: account}},
	}}

	build := func(t *testing.T, tenant *recordingTenant) (*scopedPartnerAuthz, *fiber.App) {
		t.Helper()

		authz, server := newScopedPartnerAuthz(t)

		authClient := &authMiddleware.AuthClient{Enabled: true, Address: server.URL}
		require.NoError(t, scoperesolver.Register(authClient, validations, tenant))
		require.NoError(t, declaration.WireScope(authClient, tracerembed.TracerManifest))

		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
		deps.TransactionValidationService.EXPECT().GetTransactionValidation(gomock.Any(), own).
			Return(validations.stored[own], nil).AnyTimes()

		authz.reset(map[string][]string{"accountId": {account.String()}}, false)

		return authz, deps.buildWithAuthClient(authClient)
	}

	read := func(t *testing.T, app *fiber.App, claims jwt.MapClaims) int {
		t.Helper()

		partner := jwt.MapClaims{"type": "application", "owner": "mt-org", "sub": "mt-org/mt-app", "partner": "mt-partner"}
		for k, v := range claims {
			partner[k] = v
		}

		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, partner).SignedString([]byte("mt-secret"))
		require.NoError(t, err)

		req := httptest.NewRequest(fiber.MethodGet, "/v1/validations/"+own.String(), nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	t.Run("the credential's tenant database resolves the validation", func(t *testing.T) {
		tenant := &recordingTenant{active: true}
		authz, app := build(t, tenant)

		assert.Equal(t, fiber.StatusOK, read(t, app, jwt.MapClaims{"tenantId": tenantID}))
		assert.Equal(t, []string{account.String()}, authz.asked("accountId"))
		assert.Contains(t, tenant.asked, canonical, "the database of the credential's own tenant, in canonical form")
	})

	t.Run("a credential naming no tenant is refused as unavailable, never read from the default database", func(t *testing.T) {
		tenant := &recordingTenant{active: true}
		authz, app := build(t, tenant)

		assert.Equal(t, fiber.StatusServiceUnavailable, read(t, app, nil))
		assert.Empty(t, authz.asked("accountId"))
	})

	t.Run("a credential naming a malformed tenant is refused before any database is attached", func(t *testing.T) {
		tenant := &recordingTenant{active: true}
		_, app := build(t, tenant)

		assert.Equal(t, fiber.StatusServiceUnavailable, read(t, app, jwt.MapClaims{"tenantId": "bad tenant!"}))
		assert.Empty(t, tenant.asked)
	})

	t.Run("a single-tenant deployment reads the default database", func(t *testing.T) {
		tenant := &recordingTenant{active: false}
		authz, app := build(t, tenant)

		assert.Equal(t, fiber.StatusOK, read(t, app, jwt.MapClaims{"tenantId": tenantID}))
		assert.Empty(t, tenant.asked, "no tenant database is attached")
		assert.Equal(t, []string{account.String()}, authz.asked("accountId"))
	})
}
