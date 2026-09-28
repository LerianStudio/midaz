// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	libAuth "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	producerAuthIssuer   = "https://access-manager.example.test"
	producerAuthClientID = "ledger-m2m-client"
)

// fakeKeySource serves a fixed verification key without any network access.
type fakeKeySource struct{ keys []*rsa.PublicKey }

func (f *fakeKeySource) Keys(context.Context) []*rsa.PublicKey { return f.keys }

func (f *fakeKeySource) Refresh(context.Context) error { return nil }

func (f *fakeKeySource) Close() error { return nil }

type producerAuthFixture struct {
	key *rsa.PrivateKey
	app *fiber.App
	got *context.Context
}

// testProducerAuthChain builds the producer authentication chain over a key
// generated for the test, mapping producerAuthClientID onto the ledger.
func testProducerAuthChain(t *testing.T) (*rsa.PrivateKey, []fiber.Handler) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(&fakeKeySource{keys: []*rsa.PublicKey{&key.PublicKey}}, producerAuthIssuer, true, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	return key, NewProducerAuthMiddleware(m2m, reg)
}

// signProducerToken signs an application token for producerAuthClientID;
// mutate may alter the claims before signing.
func signProducerToken(t *testing.T, key *rsa.PrivateKey, mutate func(jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{
		"type": "application",
		"sub":  "lerian/ledger-application",
		"azp":  producerAuthClientID,
		"iss":  producerAuthIssuer,
		"iat":  float64(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()),
		"exp":  float64(time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()),
	}
	if mutate != nil {
		mutate(claims)
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)

	return signed
}

func newProducerAuthFixture(t *testing.T) *producerAuthFixture {
	t.Helper()

	key, chain := testProducerAuthChain(t)

	fixture := &producerAuthFixture{key: key, got: new(context.Context)}
	fixture.app = newProducerAuthApp(chain, fixture.got)

	return fixture
}

func newProducerAuthApp(chain []fiber.Handler, got *context.Context) *fiber.App {
	app := fiber.New()

	handlers := make([]any, 0, len(chain)+1)
	for _, handler := range chain {
		handlers = append(handlers, handler)
	}

	handlers = append(handlers, func(c fiber.Ctx) error {
		*got = c.Context()

		return c.SendStatus(http.StatusCreated)
	})

	app.Post("/v1/reservations", handlers[0], handlers[1:]...)

	return app
}

func (f *producerAuthFixture) token(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()

	return signProducerToken(t, f.key, mutate)
}

func (f *producerAuthFixture) post(t *testing.T, bearer string) (int, []byte) {
	t.Helper()

	status, body, _ := f.postWithContentType(t, bearer)

	return status, body
}

func (f *producerAuthFixture) postWithContentType(t *testing.T, bearer string) (int, []byte, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := f.app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, body, resp.Header.Get("Content-Type")
}

// requireProblem asserts a problem document carrying code, whose body never
// echoes the presented token.
func requireProblem(t *testing.T, status int, body []byte, contentType string, wantStatus int, wantCode, token string) {
	t.Helper()

	require.Equal(t, wantStatus, status, string(body))
	require.Contains(t, contentType, "application/problem+json")
	require.Equal(t, wantCode, errorCode(t, body))

	if token != "" {
		require.NotContains(t, string(body), token, "the rejection never echoes the token")
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

func TestProducerAuthMiddleware_MissingTokenIsUnauthorized(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t)

	status, body, contentType := fixture.postWithContentType(t, "")
	requireProblem(t, status, body, contentType, http.StatusUnauthorized, constant.ErrTokenMissing.Error(), "")
	require.Nil(t, *fixture.got)
}

func TestProducerAuthMiddleware_UnverifiableTokenIsUnauthorized(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t)
	other := newProducerAuthFixture(t)

	for name, token := range map[string]string{
		"foreign signing key": other.token(t, nil),
		"wrong issuer":        fixture.token(t, func(c jwt.MapClaims) { c["iss"] = "https://other-issuer.example.test" }),
		"expired": fixture.token(t, func(c jwt.MapClaims) {
			c["exp"] = float64(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC).Unix())
		}),
		"malformed": "not-a-jwt",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, body, contentType := fixture.postWithContentType(t, token)
			requireProblem(t, status, body, contentType, http.StatusUnauthorized, constant.ErrInvalidToken.Error(), token)
			require.Nil(t, *fixture.got)
		})
	}
}

func TestProducerAuthMiddleware_UserTokenIsForbidden(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t)

	token := fixture.token(t, func(c jwt.MapClaims) { c["type"] = "normal-user" })

	status, body, contentType := fixture.postWithContentType(t, token)
	requireProblem(t, status, body, contentType, http.StatusForbidden, constant.ErrInsufficientPrivileges.Error(), token)
	require.Nil(t, *fixture.got)
}

func TestProducerAuthMiddleware_UnknownAuthorizedPartyIsForbidden(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t)

	for name, mutate := range map[string]func(jwt.MapClaims){
		"unmapped azp": func(c jwt.MapClaims) { c["azp"] = "someone-else" },
		"missing azp":  func(c jwt.MapClaims) { delete(c, "azp") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, body := fixture.post(t, fixture.token(t, mutate))
			require.Equal(t, http.StatusForbidden, status)
			require.Equal(t, constant.ErrInsufficientPrivileges.Error(), errorCode(t, body))
			require.Nil(t, *fixture.got)
		})
	}
}

func TestProducerAuthMiddleware_MappedAuthorizedPartyResolvesProducer(t *testing.T) {
	t.Parallel()

	fixture := newProducerAuthFixture(t)

	status, _ := fixture.post(t, fixture.token(t, nil))
	require.Equal(t, http.StatusCreated, status)
	require.NotNil(t, *fixture.got)

	producer, ok := producerauth.ProducerFromContext(*fixture.got)
	require.True(t, ok)
	require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}, producer)

	identity, ok := contextutil.GetIntegrationIdentity(*fixture.got)
	require.True(t, ok)
	require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
}

func TestProducerAuthMiddleware_DisabledVerifierWithoutOptionFailsClosed(t *testing.T) {
	t.Parallel()

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(nil, "", false, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	var got context.Context

	app := newProducerAuthApp(NewProducerAuthMiddleware(m2m, reg), &got)

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/reservations", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusForbidden, resp.StatusCode, "a request without a verified identity is never promoted to a producer")
	require.Nil(t, got)
}

func TestProducerAuthMiddleware_VerificationDisabledAttributesTheLedger(t *testing.T) {
	t.Parallel()

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(nil, "", false, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	for name, bearer := range map[string]string{
		"no token":         "",
		"unverified token": "not-a-jwt",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got context.Context

			app := newProducerAuthApp(NewProducerAuthMiddleware(m2m, reg, WithProducerVerificationDisabled()), &got)

			req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
			if bearer != "" {
				req.Header.Set("Authorization", "Bearer "+bearer)
			}

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			require.Equal(t, http.StatusCreated, resp.StatusCode)
			require.NotNil(t, got)

			producer, ok := producerauth.ProducerFromContext(got)
			require.True(t, ok)
			require.Equal(t, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}, producer)

			identity, ok := contextutil.GetIntegrationIdentity(got)
			require.True(t, ok)
			require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
		})
	}
}

func TestProducerAuthMiddleware_VerificationDisabledStillRequiresDependencies(t *testing.T) {
	t.Parallel()

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(nil, "", false, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	for name, chain := range map[string][]fiber.Handler{
		"nil authenticator": NewProducerAuthMiddleware(nil, reg, WithProducerVerificationDisabled()),
		"nil registry":      NewProducerAuthMiddleware(m2m, nil, WithProducerVerificationDisabled()),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got context.Context

			resp, err := newProducerAuthApp(chain, &got).Test(httptest.NewRequest(http.MethodPost, "/v1/reservations", nil))
			require.NoError(t, err)

			defer resp.Body.Close()

			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			require.Nil(t, got)
		})
	}
}

func TestProducerAuthMiddleware_MissingDependenciesAreUnavailable(t *testing.T) {
	t.Parallel()

	m2m, err := libAuth.NewM2MAuthenticatorWithKeySource(&fakeKeySource{}, producerAuthIssuer, true, libLog.NewNop())
	require.NoError(t, err)

	reg, err := producerauth.ParsePlatformProducers(`[{"service":"ledger","clientId":"` + producerAuthClientID + `"}]`)
	require.NoError(t, err)

	for name, chain := range map[string][]fiber.Handler{
		"nil authenticator": NewProducerAuthMiddleware(nil, reg),
		"nil registry":      NewProducerAuthMiddleware(m2m, nil),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got context.Context

			resp, err := newProducerAuthApp(chain, &got).Test(httptest.NewRequest(http.MethodPost, "/v1/reservations", nil))
			require.NoError(t, err)

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			require.Equal(t, constant.ErrContextPolicyUnavailable.Error(), errorCode(t, body))
			require.Nil(t, got)
		})
	}
}
