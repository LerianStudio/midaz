// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	libAuth "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const (
	producerAuthClientID = "ledger-m2m-client"
	// deniedApplicationSub marks a token the fake Access Manager refuses.
	deniedApplicationSub = "denied-application"
	producerAuthAPIKey   = "reservation-test-api-key-32-characters" // gitleaks:allow -- test fixture, not a credential
)

// accessManagerFake answers /v1/authorize: it refuses a token whose sub is
// deniedApplicationSub, answers 503 while down, and grants everything else.
// It records each decision's resource/action/product.
type accessManagerFake struct {
	server    *httptest.Server
	down      atomic.Bool
	mu        sync.Mutex
	decisions []map[string]any
}

func startAccessManagerFake(t *testing.T) *accessManagerFake {
	t.Helper()

	fake := &accessManagerFake{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if fake.down.Load() {
			http.Error(w, "access manager unavailable", http.StatusServiceUnavailable)
			return
		}

		var decision map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decision); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		fake.mu.Lock()
		fake.decisions = append(fake.decisions, decision)
		fake.mu.Unlock()

		claims := jwt.MapClaims{}
		_, _, err := jwt.NewParser().ParseUnverified(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), &claims)
		authorized := err == nil && claims["sub"] != deniedApplicationSub

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(libAuth.AuthResponse{Authorized: authorized})
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

func (f *accessManagerFake) recorded() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]map[string]any(nil), f.decisions...)
}

// producerAuthGuard is the reservation route guard over fake, with the API key
// enabled so a test can present it instead of a token.
func producerAuthGuard(fake *accessManagerFake, inversion bool) *middleware.AuthGuard {
	client := libAuth.NewAuthClient(fake.server.URL, true, libLog.NewNop())
	client.M2MInversionEnabled = inversion

	return middleware.NewAuthGuard(middleware.AuthGuardConfig{
		AppName: "tracer", PluginAuthEnabled: true, APIKeyEnabled: true, APIKey: producerAuthAPIKey,
	}, client)
}

func testProducerRegistry(t *testing.T) *producerauth.Registry {
	t.Helper()

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	return reg
}

// producerToken is an application token for producerAuthClientID. Its
// signature is never verified by the Tracer: the Access Manager decides it.
// mutate may alter the claims before signing.
func producerToken(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{"type": "application", "sub": "ledger-application", "azp": producerAuthClientID}
	if mutate != nil {
		mutate(claims)
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("unverified-test-signature"))
	require.NoError(t, err)

	return signed
}

type producerAuthFixture struct {
	accessManager *accessManagerFake
	app           *fiber.App
	got           *context.Context
}

func newProducerAuthFixture(t *testing.T, inversion bool) *producerAuthFixture {
	t.Helper()

	fake := startAccessManagerFake(t)
	fixture := &producerAuthFixture{accessManager: fake, got: new(context.Context)}
	fixture.app = newProducerAuthApp(NewProducerAuthMiddleware(producerAuthGuard(fake, inversion), testProducerRegistry(t)), fixture.got)

	return fixture
}

// newProducerAuthApp mounts the producerAuth chain exactly as the reservation
// routes do, in front of a handler that records the request context.
func newProducerAuthApp(producerAuth []fiber.Handler, got *context.Context) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	handlers := make([]any, 0, len(producerAuth))
	for _, handler := range producerAuth[1:] {
		handlers = append(handlers, handler)
	}

	handlers = append(handlers, func(c fiber.Ctx) error {
		*got = c.Context()

		return c.SendStatus(http.StatusCreated)
	})

	app.Post("/v1/reservations", producerAuth[0], handlers...)

	return app
}

type producerAuthResponse struct {
	status      int
	body        []byte
	contentType string
}

func (f *producerAuthFixture) post(t *testing.T, bearer string, header http.Header) producerAuthResponse {
	t.Helper()

	return postProducerAuth(t, f.app, bearer, header)
}

func postProducerAuth(t *testing.T, app *fiber.App, bearer string, header http.Header) producerAuthResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
	for name, values := range header {
		req.Header[name] = values
	}

	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return producerAuthResponse{status: resp.StatusCode, body: body, contentType: resp.Header.Get("Content-Type")}
}

// requireProblem asserts a problem document carrying code, whose body never
// echoes the presented token.
func requireProblem(t *testing.T, response producerAuthResponse, wantStatus int, wantCode, token string) {
	t.Helper()

	require.Equal(t, wantStatus, response.status, string(response.body))
	require.Contains(t, response.contentType, "application/problem+json")
	require.Equal(t, wantCode, errorCode(t, response.body))

	if token != "" {
		require.NotContains(t, string(response.body), token, "the rejection never echoes the token")
	}
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()

	var envelope struct {
		Code string `json:"code"`
	}

	require.NoError(t, json.Unmarshal(body, &envelope), string(body))

	return envelope.Code
}

func requireLedgerProducer(t *testing.T, ctx context.Context) {
	t.Helper()

	require.NotNil(t, ctx)

	producer, ok := producerauth.ProducerFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}, producer)

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	require.True(t, ok)
	require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
}

func TestProducerAuthMiddleware_MissingTokenIsUnauthorized(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t, false)

	requireProblem(t, fixture.post(t, "", nil), http.StatusUnauthorized, constant.ErrInvalidToken.Error(), "")
	require.Nil(t, *fixture.got)
	require.Empty(t, fixture.accessManager.recorded(), "a missing token never reaches the Access Manager")
}

func TestProducerAuthMiddleware_APIKeyAloneIsUnauthorized(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t, false)

	response := fixture.post(t, "", http.Header{"X-Api-Key": []string{producerAuthAPIKey}})
	requireProblem(t, response, http.StatusUnauthorized, constant.ErrInvalidToken.Error(), "")
	require.NotContains(t, string(response.body), producerAuthAPIKey)
	require.Nil(t, *fixture.got, "an API key never substitutes for a producer token")
}

func TestProducerAuthMiddleware_AccessManagerDenialIsForbidden(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t, false)
	token := producerToken(t, func(c jwt.MapClaims) { c["sub"] = deniedApplicationSub })

	requireProblem(t, fixture.post(t, token, nil), http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), token)
	require.Nil(t, *fixture.got)
	require.Len(t, fixture.accessManager.recorded(), 1, "the refusal is the Access Manager's single decision")
}

func TestProducerAuthMiddleware_AccessManagerOutageIsUnavailable(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t, false)
	fixture.accessManager.down.Store(true)
	token := producerToken(t, nil)

	requireProblem(t, fixture.post(t, token, nil), http.StatusServiceUnavailable, constant.ErrAuthorizationServiceUnavailable.Error(), token)
	require.Nil(t, *fixture.got)
}

func TestProducerAuthMiddleware_UserTokenIsForbidden(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t, false)
	token := producerToken(t, func(c jwt.MapClaims) {
		c["type"] = "normal-user"
		c["owner"] = "lerian"
	})

	requireProblem(t, fixture.post(t, token, nil), http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), token)
	require.Nil(t, *fixture.got)
	require.Len(t, fixture.accessManager.recorded(), 1, "the Access Manager authorized the user before the producer check refused it")
}

func TestProducerAuthMiddleware_LegacyUnknownCallerIsForbidden(t *testing.T) {
	t.Parallel()

	// The legacy derivation authorizes any non-user type under a fabricated
	// role, so every one of these reaches the producer check, which refuses it.
	for name, mutate := range map[string]func(jwt.MapClaims){
		"unmapped azp":    func(c jwt.MapClaims) { c["azp"] = "someone-else" },
		"missing azp":     func(c jwt.MapClaims) { delete(c, "azp") },
		"non-string azp":  func(c jwt.MapClaims) { c["azp"] = 42 },
		"padded azp":      func(c jwt.MapClaims) { c["azp"] = " " + producerAuthClientID },
		"missing type":    func(c jwt.MapClaims) { delete(c, "type") },
		"unexpected type": func(c jwt.MapClaims) { c["type"] = "service" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newProducerAuthFixture(t, false)
			token := producerToken(t, mutate)

			requireProblem(t, fixture.post(t, token, nil), http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), token)
			require.Nil(t, *fixture.got)
			require.Len(t, fixture.accessManager.recorded(), 1, "the Access Manager authorized the token before the producer check refused it")
		})
	}
}

func TestProducerAuthMiddleware_InversionUnknownCallerIsRefused(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		mutate     func(jwt.MapClaims)
		wantStatus int
		wantCode   string
		decisions  int
	}{
		"missing type":    {mutate: func(c jwt.MapClaims) { delete(c, "type") }, wantStatus: http.StatusUnauthorized, wantCode: constant.ErrInvalidToken.Error()},
		"unexpected type": {mutate: func(c jwt.MapClaims) { c["type"] = "service" }, wantStatus: http.StatusUnauthorized, wantCode: constant.ErrInvalidToken.Error()},
		"unmapped azp":    {mutate: func(c jwt.MapClaims) { c["azp"] = "someone-else" }, wantStatus: http.StatusForbidden, wantCode: constant.ErrInsufficientPrivileges.Error(), decisions: 1},
		"missing azp":     {mutate: func(c jwt.MapClaims) { delete(c, "azp") }, wantStatus: http.StatusForbidden, wantCode: constant.ErrInsufficientPrivileges.Error(), decisions: 1},
		"padded azp":      {mutate: func(c jwt.MapClaims) { c["azp"] = " " + producerAuthClientID }, wantStatus: http.StatusForbidden, wantCode: constant.ErrInsufficientPrivileges.Error(), decisions: 1},
		"non-string azp":  {mutate: func(c jwt.MapClaims) { c["azp"] = 42 }, wantStatus: http.StatusForbidden, wantCode: constant.ErrInsufficientPrivileges.Error(), decisions: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newProducerAuthFixture(t, true)
			token := producerToken(t, tc.mutate)

			requireProblem(t, fixture.post(t, token, nil), tc.wantStatus, tc.wantCode, token)
			require.Nil(t, *fixture.got)
			require.Len(t, fixture.accessManager.recorded(), tc.decisions, "lib-auth refuses an unsupported type before asking the Access Manager")
		})
	}
}

func TestProducerAuthMiddleware_MappedAuthorizedPartyResolvesProducer(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		inversion bool
		raw       bool
	}{
		"legacy derivation, bearer token": {},
		"legacy derivation, raw token":    {raw: true},
		"M2M inversion, bearer token":     {inversion: true},
		"M2M inversion, raw token":        {inversion: true, raw: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newProducerAuthFixture(t, tc.inversion)
			token := producerToken(t, nil)

			var response producerAuthResponse
			if tc.raw {
				response = fixture.post(t, "", http.Header{"Authorization": []string{token}})
			} else {
				response = fixture.post(t, token, nil)
			}

			require.Equal(t, http.StatusCreated, response.status, string(response.body))
			requireLedgerProducer(t, *fixture.got)

			// The guard publishes an actor principal for a Bearer token only;
			// whenever one is published, the producer is recorded as a system
			// actor, never as a user.
			principal, ok := contextutil.GetPrincipal(*fixture.got)
			require.Equal(t, !tc.raw, ok)

			if ok {
				require.Equal(t, contextutil.Principal{Type: string(model.ActorTypeSystem), ID: "ledger-application"}, principal)
			}

			decisions := fixture.accessManager.recorded()
			require.Len(t, decisions, 1)
			require.Equal(t, "reservations", decisions[0]["resource"])
			require.Equal(t, "post", decisions[0]["action"])
		})
	}
}

func TestProducerAuthMiddleware_VerificationDisabledAttributesTheLedger(t *testing.T) {
	t.Parallel()

	// The unverified chain mounts no guard: an API-key guard would demand an
	// X-API-Key the Ledger never sends.
	apiKeyGuard := middleware.NewAuthGuard(middleware.AuthGuardConfig{AppName: "tracer", APIKeyEnabled: true, APIKey: producerAuthAPIKey}, libAuth.NewAuthClient("", false, libLog.NewNop()))

	for name, tc := range map[string]struct {
		guard  *middleware.AuthGuard
		bearer string
	}{
		"no token":          {guard: apiKeyGuard},
		"unverified token":  {guard: apiKeyGuard, bearer: "not-a-jwt"},
		"application token": {guard: apiKeyGuard, bearer: "application"},
		"no guard":          {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got context.Context

			app := newProducerAuthApp(NewProducerAuthMiddleware(tc.guard, testProducerRegistry(t), WithProducerVerificationDisabled()), &got)

			bearer := tc.bearer
			if bearer == "application" {
				bearer = producerToken(t, nil)
			}

			response := postProducerAuth(t, app, bearer, nil)
			require.Equal(t, http.StatusCreated, response.status, string(response.body))
			requireLedgerProducer(t, got)

			_, ok := contextutil.GetPrincipal(got)
			require.False(t, ok, "no guard ran, so no actor principal is published and the audit actor falls back to system")
		})
	}
}

func TestProducerAuthMiddleware_UnverifiedRequestIsNeverPromoted(t *testing.T) {
	t.Parallel()

	// Plugin auth off lets the guard pass everything; without the explicit
	// option the producer check still demands an authorized application token.
	guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{AppName: "tracer"}, libAuth.NewAuthClient("", false, libLog.NewNop()))

	var got context.Context

	app := newProducerAuthApp(NewProducerAuthMiddleware(guard, testProducerRegistry(t)), &got)

	response := postProducerAuth(t, app, "", nil)
	requireProblem(t, response, http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), "")
	require.Nil(t, got)
}

func TestProducerAuthMiddleware_MissingDependencyIsUnavailable(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		noGuard    bool
		noRegistry bool
		opts       []ProducerAuthOption
	}{
		"verified without a registry":              {noRegistry: true},
		"verification disabled without a registry": {noRegistry: true, opts: []ProducerAuthOption{WithProducerVerificationDisabled()}},
		"verified without a guard":                 {noGuard: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newProducerAuthFixture(t, false)

			guard := producerAuthGuard(fixture.accessManager, false)
			if tc.noGuard {
				guard = nil
			}

			reg := testProducerRegistry(t)
			if tc.noRegistry {
				reg = nil
			}

			var got context.Context

			app := newProducerAuthApp(NewProducerAuthMiddleware(guard, reg, tc.opts...), &got)

			response := postProducerAuth(t, app, producerToken(t, nil), nil)
			requireProblem(t, response, http.StatusServiceUnavailable, constant.ErrContextPolicyUnavailable.Error(), "")
			require.Nil(t, got)
			require.Empty(t, fixture.accessManager.recorded(), "an incomplete chain never consults the Access Manager")
		})
	}
}
