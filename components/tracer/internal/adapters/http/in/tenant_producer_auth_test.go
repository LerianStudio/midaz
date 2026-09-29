// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const (
	// mtTokenTenant is the tenant a tenant-manager application token carries,
	// in the dashless form tenant-manager writes; mtTokenTenantDashed is the
	// same tenant with dashes.
	mtTokenTenant       = "0195d3b45a0170008000000000000007"
	mtTokenTenantDashed = "0195d3b4-5a01-7000-8000-000000000007"
)

// tenantProducerToken is the access token of the per-tenant M2M application
// tenant-manager provisions for the ledger: a random client id and the
// platform attributes only tenant-manager can write. mutate may alter it.
func tenantProducerToken(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{
		"type": "application", "sub": "admin/ledger-m2m-tracer-" + mtTokenTenant, "azp": "7f3c1e9a0b2d4c6e8f01",
		"name": "ledger-m2m-tracer-" + mtTokenTenant, "owner": "admin",
		"tenantId": mtTokenTenant, "tenantSlug": "acme", "isInternal": "true", "sourceService": "ledger",
	}
	if mutate != nil {
		mutate(claims)
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("unverified-test-signature"))
	require.NoError(t, err)

	return signed
}

// tokenTenantFixture records the association lookups and pool bindings the
// multi-tenant reservation chain makes, in order.
type tokenTenantFixture struct {
	accessManager *accessManagerFake
	lookupErr     error
	bindErr       error
	// bindTenant overrides the tenant the binder binds; empty binds the tenant
	// the lib-commons middleware would read, the canonical token tenant.
	bindTenant string
	noPool     bool
	downstream error
	pool       dbresolver.DB

	mu    sync.Mutex
	steps []string
	got   context.Context
}

func newTokenTenantFixture(t *testing.T) *tokenTenantFixture {
	t.Helper()

	return &tokenTenantFixture{accessManager: startAccessManagerFake(t), pool: mwStubDB(t)}
}

func (f *tokenTenantFixture) record(step string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.steps = append(f.steps, step)
}

func (f *tokenTenantFixture) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.steps...)
}

func (f *tokenTenantFixture) authorizer() *producerauth.TenantAuthorizer {
	return producerauth.NewTenantAuthorizer(func(_ context.Context, tenantID, service string) error {
		f.record("lookup:" + service + ":" + tenantID)

		return f.lookupErr
	}, true)
}

// bind stands in for the lib-commons WithTenantDB: it binds the tenant it
// would read from the token and continues, or returns its refusal.
func (f *tokenTenantFixture) bind(c fiber.Ctx) error {
	tenantID := f.bindTenant
	if tenantID == "" {
		tenantID, _ = c.Locals(tokenTenantLocal{}).(string)
	}

	f.record("bind:" + tenantID)

	if f.bindErr != nil {
		return f.bindErr
	}

	ctx := tmcore.ContextWithTenantID(c.Context(), tenantID)
	if !f.noPool {
		ctx = tmcore.ContextWithPG(ctx, f.pool)
	}

	c.SetContext(ctx)

	return c.Next()
}

func (f *tokenTenantFixture) app(inversion bool, authz *producerauth.TenantAuthorizer, bind fiber.Handler) *fiber.App {
	chain := append(NewTenantProducerAuthMiddleware(producerAuthGuard(f.accessManager, inversion)), reservationTokenTenantMiddleware(authz, bind)...)

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	handlers := make([]any, 0, len(chain))
	for _, handler := range chain[1:] {
		handlers = append(handlers, handler)
	}

	handlers = append(handlers, func(c fiber.Ctx) error {
		f.mu.Lock()
		f.got = c.Context()
		f.mu.Unlock()

		if f.downstream != nil {
			return f.downstream
		}

		return c.SendStatus(http.StatusCreated)
	})

	app.Post("/v1/reservations", chain[0], handlers...)

	return app
}

func (f *tokenTenantFixture) context() context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.got
}

func requestedTenant(tenantID string) http.Header {
	return http.Header{seamtenant.HeaderName: []string{tenantID}}
}

func TestTenantProducerAuth_AdmitsTheTenantManagerApplication(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		inversion bool
		header    http.Header
	}{
		"legacy derivation, no requested tenant":     {},
		"M2M inversion, no requested tenant":         {inversion: true},
		"requested tenant equal to the token tenant": {header: requestedTenant(mtTokenTenant)},
		"requested tenant with dashes":               {header: requestedTenant(mtTokenTenantDashed)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newTokenTenantFixture(t)
			app := fixture.app(tc.inversion, fixture.authorizer(), fixture.bind)

			response := postProducerAuth(t, app, tenantProducerToken(t, nil), tc.header)
			require.Equal(t, http.StatusCreated, response.status, string(response.body))

			ctx := fixture.context()
			requireLedgerProducer(t, ctx)
			require.Equal(t, mtTokenTenant, tmcore.GetTenantIDContext(ctx), "the tenant is the token tenant")
			require.NotNil(t, tmcore.GetPGContext(ctx))
			require.Equal(t, []string{"lookup:ledger:" + mtTokenTenant, "bind:" + mtTokenTenant}, fixture.recorded(),
				"the ledger association is confirmed before the tenant pool is bound")

			principal, ok := contextutil.GetPrincipal(ctx)
			require.True(t, ok)
			require.Equal(t, string(model.ActorTypeSystem), principal.Type, "a producer is never a user")

			decisions := fixture.accessManager.recorded()
			require.Len(t, decisions, 1)
			require.Equal(t, "reservations", decisions[0]["resource"])
			require.Equal(t, "post", decisions[0]["action"])
		})
	}
}

func TestTenantProducerAuth_RefusesTokensWithoutTheTenantManagerClaims(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		mutate func(jwt.MapClaims)
		header http.Header
	}{
		"user token":                   {mutate: func(c jwt.MapClaims) { c["type"] = "normal-user"; c["owner"] = "lerian" }},
		"missing isInternal":           {mutate: func(c jwt.MapClaims) { delete(c, "isInternal") }},
		"isInternal false":             {mutate: func(c jwt.MapClaims) { c["isInternal"] = "false" }},
		"isInternal as a bool":         {mutate: func(c jwt.MapClaims) { c["isInternal"] = true }},
		"other sourceService":          {mutate: func(c jwt.MapClaims) { c["sourceService"] = "fees" }},
		"flowker sourceService":        {mutate: func(c jwt.MapClaims) { c["sourceService"] = "flowker" }},
		"missing sourceService":        {mutate: func(c jwt.MapClaims) { delete(c, "sourceService") }},
		"padded sourceService":         {mutate: func(c jwt.MapClaims) { c["sourceService"] = "ledger " }},
		"missing tenantId":             {mutate: func(c jwt.MapClaims) { delete(c, "tenantId") }},
		"malformed tenantId":           {mutate: func(c jwt.MapClaims) { c["tenantId"] = "../tenant" }},
		"requested tenant differs":     {header: requestedTenant("0195d3b45a0170008000000000000008")},
		"requested tenant malformed":   {header: requestedTenant("tenant a")},
		"azp-only roster application":  {mutate: func(c jwt.MapClaims) { delete(c, "isInternal"); delete(c, "sourceService"); delete(c, "tenantId") }},
		"requested tenant, user token": {mutate: func(c jwt.MapClaims) { c["type"] = "normal-user"; c["owner"] = "lerian" }, header: requestedTenant(mtTokenTenant)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newTokenTenantFixture(t)
			app := fixture.app(false, fixture.authorizer(), fixture.bind)
			token := tenantProducerToken(t, tc.mutate)

			requireProblem(t, postProducerAuth(t, app, token, tc.header), http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), token)
			require.Nil(t, fixture.context())
			require.Empty(t, fixture.recorded(), "a refused token reaches neither the tenant-manager nor a tenant pool")
			require.Len(t, fixture.accessManager.recorded(), 1, "the Access Manager authorized the token before the claim checks refused it")
		})
	}
}

func TestTenantProducerAuth_GuardRefusalsComeFirst(t *testing.T) {
	t.Parallel()

	fixture := newTokenTenantFixture(t)
	app := fixture.app(false, fixture.authorizer(), fixture.bind)

	requireProblem(t, postProducerAuth(t, app, "", requestedTenant(mtTokenTenant)), http.StatusUnauthorized, constant.ErrInvalidToken.Error(), "")

	denied := tenantProducerToken(t, func(c jwt.MapClaims) { c["sub"] = deniedApplicationSub })
	requireProblem(t, postProducerAuth(t, app, denied, nil), http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), denied)

	fixture.accessManager.down.Store(true)

	token := tenantProducerToken(t, nil)
	requireProblem(t, postProducerAuth(t, app, token, nil), http.StatusServiceUnavailable, constant.ErrAuthorizationServiceUnavailable.Error(), token)
	require.Empty(t, fixture.recorded())
}

func TestReservationTokenTenant_ClassifiesTheLedgerAssociation(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		lookupErr error
		status    int
		code      string
	}{
		"not associated with the ledger": {lookupErr: fmt.Errorf("tenant is not active for ledger: %w", tmcore.ErrTenantNotFound), status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"association access denied":      {lookupErr: fmt.Errorf("denied: %w", tmcore.ErrTenantServiceAccessDenied), status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"tenant-manager unavailable":     {lookupErr: errors.New("active tenant list unavailable"), status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error()},
		"caller went away":               {lookupErr: context.Canceled, status: http.StatusServiceUnavailable, code: constant.ErrContextCancelled.Error()},
		"deadline passed":                {lookupErr: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: constant.ErrValidationTimeout.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newTokenTenantFixture(t)
			fixture.lookupErr = tc.lookupErr
			app := fixture.app(false, fixture.authorizer(), fixture.bind)

			requireProblem(t, postProducerAuth(t, app, tenantProducerToken(t, nil), nil), tc.status, tc.code, "")
			require.Equal(t, []string{"lookup:ledger:" + mtTokenTenant}, fixture.recorded(), "no pool is bound for an unauthorized tenant")
			require.Nil(t, fixture.context())
		})
	}
}

func TestReservationTokenTenant_ClassifiesBinderRefusals(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		bindErr error
		status  int
		code    string
	}{
		"tenant not found":            {bindErr: fmt.Errorf("resolve: %w", tmcore.ErrTenantNotFound), status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"tracer association denied":   {bindErr: fmt.Errorf("resolve: %w", tmcore.ErrTenantServiceAccessDenied), status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"tracer association paused":   {bindErr: &tmcore.TenantSuspendedError{TenantID: mtTokenTenant, Status: "suspended"}, status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"missing tenantId claim":      {bindErr: tmcore.ErrMissingTenantIDClaim, status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"invalid token":               {bindErr: tmcore.ErrInvalidAuthorizationToken, status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
		"circuit breaker open":        {bindErr: fmt.Errorf("resolve: %w", tmcore.ErrCircuitBreakerOpen), status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error()},
		"service not configured":      {bindErr: fmt.Errorf("resolve: %w", tmcore.ErrServiceNotConfigured), status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error()},
		"database unreachable":        {bindErr: errors.New("dial tcp: connection refused"), status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error()},
		"caller went away":            {bindErr: fmt.Errorf("resolve: %w", context.Canceled), status: http.StatusServiceUnavailable, code: constant.ErrContextCancelled.Error()},
		"deadline passed on the dial": {bindErr: fmt.Errorf("resolve: %w", context.DeadlineExceeded), status: http.StatusGatewayTimeout, code: constant.ErrValidationTimeout.Error()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newTokenTenantFixture(t)
			fixture.bindErr = tc.bindErr
			app := fixture.app(false, fixture.authorizer(), fixture.bind)

			requireProblem(t, postProducerAuth(t, app, tenantProducerToken(t, nil), nil), tc.status, tc.code, "")
			require.Equal(t, []string{"lookup:ledger:" + mtTokenTenant, "bind:" + mtTokenTenant}, fixture.recorded())
			require.Nil(t, fixture.context())
		})
	}
}

func TestReservationTokenTenant_PassesDownstreamErrorsThrough(t *testing.T) {
	t.Parallel()

	fixture := newTokenTenantFixture(t)
	// An error the binder refusal classifier would answer with 403 0043.
	fixture.downstream = fmt.Errorf("handler outcome: %w", tmcore.ErrTenantNotFound)
	app := fixture.app(false, fixture.authorizer(), fixture.bind)

	response := postProducerAuth(t, app, tenantProducerToken(t, nil), nil)
	requireProblem(t, response, http.StatusInternalServerError, constant.ErrInternalServer.Error(), "")
	require.NotContains(t, string(response.body), "handler outcome", "the downstream cause never reaches the response")
	require.NotNil(t, fixture.context(), "the handler ran")
}

func TestReservationTokenTenant_WiringDefectsFailClosed(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		configure func(*tokenTenantFixture)
		authz     func(*tokenTenantFixture) *producerauth.TenantAuthorizer
		noBind    bool
	}{
		"binder binds another tenant": {configure: func(f *tokenTenantFixture) { f.bindTenant = "0195d3b45a0170008000000000000009" }},
		"binder binds no pool":        {configure: func(f *tokenTenantFixture) { f.noPool = true }},
		"inactive authorizer": {authz: func(*tokenTenantFixture) *producerauth.TenantAuthorizer {
			return producerauth.NewTenantAuthorizer(nil, false)
		}},
		"nil authorizer": {authz: func(*tokenTenantFixture) *producerauth.TenantAuthorizer { return nil }},
		"no binder":      {noBind: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newTokenTenantFixture(t)
			if tc.configure != nil {
				tc.configure(fixture)
			}

			authz := fixture.authorizer()
			if tc.authz != nil {
				authz = tc.authz(fixture)
			}

			var bind fiber.Handler = fixture.bind
			if tc.noBind {
				bind = nil
			}

			requireProblem(t, postProducerAuth(t, fixture.app(false, authz, bind), tenantProducerToken(t, nil), nil),
				http.StatusServiceUnavailable, constant.ErrContextPolicyUnavailable.Error(), "")
			require.Nil(t, fixture.context())
		})
	}
}

func TestReservationTokenTenant_RequiresTheTokenTenantStep(t *testing.T) {
	t.Parallel()

	fixture := newTokenTenantFixture(t)

	// Mounted without the multi-tenant producer check, no token tenant is
	// known, so the chain refuses before any lookup.
	chain := append([]fiber.Handler{injectProducer}, reservationTokenTenantMiddleware(fixture.authorizer(), fixture.bind)...)
	app := newProducerAuthApp(chain, new(context.Context))

	requireProblem(t, postProducerAuth(t, app, tenantProducerToken(t, nil), requestedTenant(mtTokenTenant)),
		http.StatusServiceUnavailable, constant.ErrContextPolicyUnavailable.Error(), "")
	require.Empty(t, fixture.recorded())
}
