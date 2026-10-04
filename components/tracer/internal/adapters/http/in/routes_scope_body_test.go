// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// TestTracerBodyScope_ValidationSubmission drives POST /v1/validations through the real
// guard with a partner credential, against an authorization service that allows only
// the A1 account, segment S1, portfolio P1 and merchant M1.
func TestTracerBodyScope_ValidationSubmission(t *testing.T) {
	a1, a2 := uuid.NewString(), uuid.NewString()
	s1, p1, m1 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	allowed := map[string]string{"accountId": a1, "segmentId": s1, "portfolioId": p1, "merchantId": m1}

	var (
		mu    sync.Mutex
		asked []map[string]string
	)

	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Attributes map[string]string `json:"attributes"`
		}

		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		mu.Lock()
		asked = append(asked, body.Attributes)
		mu.Unlock()

		ok := true
		for name, value := range body.Attributes {
			if allowed[name] != value {
				ok = false
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorized":` + map[bool]string{true: "true", false: "false"}[ok] + `}`))
	}))
	t.Cleanup(authz.Close)

	authClient := &authMiddleware.AuthClient{Enabled: true, Address: authz.URL}
	wireTracerScope(t, authClient, nil)

	deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
	deps.ValidationService.EXPECT().Validate(gomock.Any(), gomock.Any()).Return(nil, errors.New("not reached in this test")).AnyTimes()

	app := deps.buildWithAuthClient(authClient)

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application", "owner": "scope-org", "sub": "scope-org/scope-app", "partner": "scope-partner",
	}).SignedString([]byte("scope-secret"))
	require.NoError(t, err)

	submit := func(body string) (int, string) {
		t.Helper()

		mu.Lock()
		asked = nil
		mu.Unlock()

		req := httptest.NewRequest(fiber.MethodPost, "/v1/validations", strings.NewReader(body))
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		raw, _ := io.ReadAll(resp.Body)

		return resp.StatusCode, string(raw)
	}

	ctx := func(name, value string) string { return `"` + name + `":{"` + name + `Id":"` + value + `"}` }

	t.Run("the allowed account passes the guard", func(t *testing.T) {
		status, body := submit(`{` + ctx("account", a1) + `}`)
		assert.NotEqual(t, fiber.StatusForbidden, status)
		assert.NotContains(t, body, "scope field", "the guard passed it on; the handler answers what remains")
		require.Len(t, asked, 1)
		assert.Equal(t, map[string]string{"accountId": a1}, asked[0], "optional context left out is asked without it")
	})

	t.Run("another account is refused", func(t *testing.T) {
		status, _ := submit(`{` + ctx("account", a2) + `}`)
		assert.Equal(t, fiber.StatusForbidden, status)
	})

	t.Run("allowed context passes, each out-of-scope context is refused", func(t *testing.T) {
		status, _ := submit(`{` + ctx("account", a1) + `,` + ctx("segment", s1) + `,` + ctx("portfolio", p1) + `,` + ctx("merchant", m1) + `}`)
		assert.NotEqual(t, fiber.StatusForbidden, status)
		require.Len(t, asked, 1)
		assert.Equal(t, allowed, asked[0])

		for _, name := range []string{"segment", "portfolio", "merchant"} {
			status, _ := submit(`{` + ctx("account", a1) + `,` + ctx(name, uuid.NewString()) + `}`)
			assert.Equalf(t, fiber.StatusForbidden, status, "an out-of-scope %s must be refused", name)
		}
	})

	t.Run("a submission naming no account is refused naming the field", func(t *testing.T) {
		status, body := submit(`{}`)
		assert.Equal(t, fiber.StatusBadRequest, status)
		assert.Contains(t, body, `scope field \"account\" is missing`)
		assert.Empty(t, asked, "no authorization call for an unreadable body")
	})
}
