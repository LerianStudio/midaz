// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libAuth "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func contextReservationRoutesDeps(t *testing.T) RoutesDeps {
	t.Helper()

	ctrl := gomock.NewController(t)
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}

	handler, err := NewContextReservationHandler(mocks.NewMockContextReserveAdmitter(ctrl), mocks.NewMockContextReserveCompleter(ctrl), mocks.NewMockContextReserveIDCompleter(ctrl), bounds, 65536, 100)
	require.NoError(t, err)

	logger := testutil.NewMockLogger()

	return RoutesDeps{
		Logger:                       logger,
		Telemetry:                    &libOtel.Telemetry{TelemetryConfig: libOtel.TelemetryConfig{ServiceName: "tracer-test", Logger: logger}},
		HealthChecker:                &HealthChecker{},
		RuleService:                  NewMockRuleService(ctrl),
		LimitService:                 NewMockLimitService(ctrl),
		ValidationService:            mocks.NewMockValidationService(ctrl),
		ContextReservation:           handler,
		TransactionValidationService: mocks.NewMockTransactionValidationService(ctrl),
		AuditEventService:            NewMockAuditEventService(ctrl),
		DashboardService:             &dashboardServiceStub{},
		Guard:                        middleware.NewAuthGuard(middleware.AuthGuardConfig{}, libAuth.NewAuthClient("", false, libLog.NewNop())),
		Clock:                        clock.New(),
	}
}

func TestNewRoutes_ContextReservationRequiresProducerVerifier(t *testing.T) {
	_, err := NewRoutes(contextReservationRoutesDeps(t))
	require.ErrorContains(t, err, "context reservations require verified producer identity")
}

func TestNewRoutes_MultiTenantContextReservationRequiresTenantAuthorizer(t *testing.T) {
	key, _ := testProducerAuthChain(t)

	for name, authorizer := range map[string]*producerauth.TenantAuthorizer{
		"no authorizer":            nil,
		"single-tenant authorizer": producerauth.NewTenantAuthorizer(nil, false),
	} {
		t.Run(name, func(t *testing.T) {
			deps := withProducerVerifier(t, contextReservationRoutesDeps(t), key)
			deps.MultiTenantEnabled = true
			deps.ContextReservationTenants = authorizer

			app, err := NewRoutes(deps)
			require.ErrorContains(t, err, "multi-tenant context reservations require the producer tenant authorizer",
				"multi-tenancy refuses an inactive authorizer whether or not a tenant pool manager is wired")
			require.Nil(t, app)
		})
	}
}

// withProducerVerifier completes deps with a producer verifier that trusts key
// and maps producerAuthClientID onto the ledger producer.
func withProducerVerifier(t *testing.T, deps RoutesDeps, key *rsa.PrivateKey) RoutesDeps {
	t.Helper()

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(&fakeKeySource{keys: []*rsa.PublicKey{&key.PublicKey}}, producerAuthIssuer, true, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	deps.ContextReservationM2M = m2m
	deps.ContextReservationProducers = reg

	return deps
}

func TestNewRoutes_ContextReservationRoutesAuthenticateProducer(t *testing.T) {
	key, _ := testProducerAuthChain(t)

	app, err := NewRoutes(withProducerVerifier(t, contextReservationRoutesDeps(t), key))
	require.NoError(t, err)

	for _, path := range []string{
		"/v1/reservations",
		"/v1/reservations/transaction/0195d3b4-5a01-7000-8000-000000000001/confirm",
		"/v1/reservations/transaction/0195d3b4-5a01-7000-8000-000000000001/release",
		"/v1/reservations/0195d3b4-5a01-7000-8000-000000000001/confirm",
		"/v1/reservations/0195d3b4-5a01-7000-8000-000000000001/release",
	} {
		t.Run(path, func(t *testing.T) {
			for name, tc := range map[string]struct {
				token string
				want  int
				code  string
			}{
				"no token":     {want: http.StatusUnauthorized, code: constant.ErrTokenMissing.Error()},
				"invalid":      {token: "not-a-jwt", want: http.StatusUnauthorized, code: constant.ErrInvalidToken.Error()},
				"user token":   {token: signProducerToken(t, key, func(c jwt.MapClaims) { c["type"] = "normal-user" }), want: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
				"unmapped azp": {token: signProducerToken(t, key, func(c jwt.MapClaims) { c["azp"] = "someone-else" }), want: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error()},
			} {
				t.Run(name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
					req.Header.Set("Content-Type", "application/json")

					if tc.token != "" {
						req.Header.Set("Authorization", "Bearer "+tc.token)
					}

					resp, err := app.Test(req)
					require.NoError(t, err)

					defer resp.Body.Close()

					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)

					require.Equal(t, tc.want, resp.StatusCode, string(body))
					require.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json", "the declared problem media type")
					require.Equal(t, tc.code, errorCode(t, body))
				})
			}
		})
	}
}

func TestNewRoutes_ContextReservationProducerVerificationDisabled(t *testing.T) {
	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(nil, "", false, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		unverified bool
		want       int
	}{
		"explicitly disabled attributes the ledger":  {unverified: true, want: http.StatusBadRequest},
		"verifier off without the flag fails closed": {unverified: false, want: http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			deps := contextReservationRoutesDeps(t)
			deps.ContextReservationM2M = m2m
			deps.ContextReservationProducers = reg
			deps.ContextReservationUnverifiedProducers = tc.unverified

			app, err := NewRoutes(deps)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/v1/reservations", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			require.Equal(t, tc.want, resp.StatusCode)
		})
	}
}
