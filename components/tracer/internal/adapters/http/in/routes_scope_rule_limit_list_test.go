// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestTracerListScope_RulesAndLimits drives GET /v1/rules and GET /v1/limits through
// the real guard and the embedded manifest with a partner credential, against an
// authorization service whose answer each row sets.
func TestTracerListScope_RulesAndLimits(t *testing.T) {
	var (
		mu     sync.Mutex
		answer string
		asked  [][]string
	)

	authz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")

		var body struct {
			Filter []string `json:"filter"`
		}

		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		mu.Lock()
		asked = append(asked, body.Filter)
		reply := answer
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(authz.Close)

	authClient := &authMiddleware.AuthClient{Enabled: true, Address: authz.URL}
	wireTracerScope(t, authClient, nil)

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "application", "owner": "scope-org", "sub": "scope-org/scope-app", "partner": "scope-partner",
	}).SignedString([]byte("scope-secret"))
	require.NoError(t, err)

	list := func(app *fiber.App, path, reply string) int {
		t.Helper()

		mu.Lock()
		answer, asked = reply, nil
		mu.Unlock()

		req := httptest.NewRequest(fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		return resp.StatusCode
	}

	ownRule, ownLimit := uuid.New(), uuid.New()

	t.Run("a ruleId partner lists only its own rules", func(t *testing.T) {
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
		deps.RuleService.EXPECT().ListRules(gomock.Any(), gomock.Cond(func(f *model.ListRulesFilter) bool {
			return assert.Equal(t, pkgHTTP.ScopeConfinement{"ruleId": {ownRule}}, f.Scope)
		})).Return(&model.ListRulesResult{}, nil)

		status := list(deps.buildWithAuthClient(authClient), "/v1/rules", `{"authorized":true,"allowed":{"ruleId":["`+ownRule.String()+`"]}}`)
		assert.Equal(t, fiber.StatusOK, status)
		assert.Equal(t, [][]string{{"ruleId"}}, asked, "the question asks for the rules the partner may see")
	})

	t.Run("a limitId partner lists only its own limits", func(t *testing.T) {
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
		deps.LimitService.EXPECT().ListLimits(gomock.Any(), gomock.Cond(func(f *model.ListLimitsFilter) bool {
			return assert.Equal(t, pkgHTTP.ScopeConfinement{"limitId": {ownLimit}}, f.Scope)
		})).Return(&model.ListLimitsResult{}, nil)

		status := list(deps.buildWithAuthClient(authClient), "/v1/limits", `{"authorized":true,"allowed":{"limitId":["`+ownLimit.String()+`"]}}`)
		assert.Equal(t, fiber.StatusOK, status)
		assert.Equal(t, [][]string{{"limitId"}}, asked, "the question asks for the limits the partner may see")
	})

	t.Run("an empty allowed list lists nothing", func(t *testing.T) {
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
		deps.RuleService.EXPECT().ListRules(gomock.Any(), gomock.Cond(func(f *model.ListRulesFilter) bool {
			return assert.True(t, f.Scope.ListsNothing())
		})).Return(&model.ListRulesResult{}, nil)

		assert.Equal(t, fiber.StatusOK, list(deps.buildWithAuthClient(authClient), "/v1/rules", `{"authorized":true,"allowed":{"ruleId":[]}}`))
	})

	t.Run("an unrestricted partner lists every rule and every limit", func(t *testing.T) {
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})
		deps.RuleService.EXPECT().ListRules(gomock.Any(), gomock.Cond(func(f *model.ListRulesFilter) bool {
			return assert.Nil(t, f.Scope, "an unrestricted answer confines nothing")
		})).Return(&model.ListRulesResult{}, nil)
		deps.LimitService.EXPECT().ListLimits(gomock.Any(), gomock.Cond(func(f *model.ListLimitsFilter) bool {
			return assert.Nil(t, f.Scope, "an unrestricted answer confines nothing")
		})).Return(&model.ListLimitsResult{}, nil)

		app := deps.buildWithAuthClient(authClient)
		assert.Equal(t, fiber.StatusOK, list(app, "/v1/rules", `{"authorized":true,"unrestricted":true}`))
		assert.Equal(t, fiber.StatusOK, list(app, "/v1/limits", `{"authorized":true,"unrestricted":true}`))
	})

	t.Run("the same partner without a grant is refused", func(t *testing.T) {
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})

		app := deps.buildWithAuthClient(authClient)
		assert.Equal(t, fiber.StatusForbidden, list(app, "/v1/rules", `{"authorized":false,"unrestricted":true}`))
		assert.Equal(t, fiber.StatusForbidden, list(app, "/v1/limits", `{"authorized":false}`))
	})

	t.Run("a grant neither unrestricted nor carrying allowed values is refused, never served unconfined", func(t *testing.T) {
		deps := newTestRouterDeps(t, middleware.AuthGuardConfig{PluginAuthEnabled: true, AppName: constant.ApplicationName})

		app := deps.buildWithAuthClient(authClient)
		assert.Equal(t, fiber.StatusForbidden, list(app, "/v1/rules", `{"authorized":true}`))
		assert.Equal(t, fiber.StatusForbidden, list(app, "/v1/limits", `{"authorized":true}`))
	})
}
