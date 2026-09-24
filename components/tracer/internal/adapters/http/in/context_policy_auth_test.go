// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	auth "github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestContextPolicyRoutePermissions(t *testing.T) {
	id := testutil.MustDeterministicUUID(66001)
	for _, route := range []struct{ method, path, resource, action, body string }{
		{http.MethodPost, "/v1/policies", "policies", "post", `{"id":"` + id.String() + `","revision":1,"defaultDecision":"DENY","rules":[]}`},
		{http.MethodGet, "/v1/policies/" + id.String() + "/revisions/1", "policies", "get", ""},
		{http.MethodPut, "/v1/policy-bindings", "policy-bindings", "put", `{"integrationId":"producer","contextId":"context","policyId":"` + id.String() + `","policyRevision":1}`},
		{http.MethodGet, "/v1/policy-bindings?integrationId=producer&contextId=context", "policy-bindings", "get", ""},
	} {
		for _, allowed := range []bool{false, true} {
			t.Run(route.method+route.path+map[bool]string{true: " allowed", false: " denied"}[allowed], func(t *testing.T) {
				requests := make(chan map[string]any, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						w.WriteHeader(http.StatusOK)
						return
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					requests <- body
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(auth.AuthResponse{Authorized: allowed})
				}))
				t.Cleanup(server.Close)
				client := auth.NewAuthClient(server.URL, true, libLog.NewNop())
				guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{AppName: "tracer", PluginAuthEnabled: true, APIKeyEnabled: true, APIKey: "validation-key"}, client)
				service := NewMockContextPolicyAdminService(gomock.NewController(t))
				if allowed {
					switch route.resource + ":" + route.action {
					case "policies:post":
						service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ model.ContextPolicy) error {
							p, ok := contextutil.GetPrincipal(ctx)
							require.True(t, ok)
							require.Equal(t, "policy-admin", p.ID)
							return nil
						})
					case "policies:get":
						service.EXPECT().GetRevision(gomock.Any(), gomock.Any()).Return(&model.ContextPolicy{ID: id, Revision: 1, DefaultDecision: model.DecisionDeny}, nil)
					case "policy-bindings:put":
						service.EXPECT().Bind(gomock.Any(), gomock.Any(), gomock.Any(), nil).Return(&model.PolicyBindingState{Policy: model.PolicyRevision{ID: id, Revision: 1}, Version: 1}, nil)
					case "policy-bindings:get":
						service.EXPECT().GetBinding(gomock.Any(), gomock.Any()).Return(&model.PolicyBindingState{Policy: model.PolicyRevision{ID: id, Revision: 1}, Version: 1}, nil)
					}
				}
				app, _ := contextPolicyTestApp(t, service, guard)
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"type": "normal-user", "sub": "policy-admin", "owner": "tenant-owner"}).SignedString([]byte("test-signature-key"))
				require.NoError(t, err)
				req := httptest.NewRequest(route.method, route.path, bytes.NewBufferString(route.body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("X-API-Key", "validation-key")
				response, err := app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)
				defer response.Body.Close()
				expected := http.StatusForbidden
				if allowed {
					expected = http.StatusOK
					if route.method == http.MethodPost {
						expected = http.StatusCreated
					}
				}
				require.Equal(t, expected, response.StatusCode)
				select {
				case request := <-requests:
					require.Equal(t, route.resource, request["resource"])
					require.Equal(t, route.action, request["action"])
					require.Equal(t, "tracer", request["product"])
				default:
					t.Fatal("Access Manager was not consulted")
				}
			})
		}
	}
}

func TestContextPolicyRejectsAuthBypasses(t *testing.T) {
	for _, scenario := range []string{"auth off", "client disabled", "address missing", "only api key", "legacy application", "unknown type", "spaced type", "spaced subject", "application no product"} {
		t.Run(scenario, func(t *testing.T) {
			service := NewMockContextPolicyAdminService(gomock.NewController(t))
			client := auth.NewAuthClient("http://127.0.0.1:1", true, libLog.NewNop())
			client.M2MInversionEnabled = false
			cfg := middleware.AuthGuardConfig{AppName: "tracer", PluginAuthEnabled: true, APIKeyEnabled: true, APIKey: "validation-key"}
			expected := http.StatusForbidden
			switch scenario {
			case "auth off":
				cfg.PluginAuthEnabled = false
				expected = http.StatusServiceUnavailable
			case "client disabled":
				client.Enabled = false
				expected = http.StatusServiceUnavailable
			case "address missing":
				client.Address = ""
				expected = http.StatusServiceUnavailable
			case "only api key", "spaced subject":
				expected = http.StatusUnauthorized
			case "application no product":
				client.M2MInversionEnabled = true
				client.ForwardM2MProduct = false
			}
			app, _ := contextPolicyTestApp(t, service, middleware.NewAuthGuard(cfg, client))
			req := httptest.NewRequest(http.MethodGet, "/v1/policy-bindings?integrationId=producer&contextId=context", nil)
			req.Header.Set("X-API-Key", "validation-key")
			if scenario != "only api key" {
				kind := "application"
				if scenario == "unknown type" {
					kind = "unknown"
				}
				if scenario == "spaced type" {
					kind = " normal-user "
				}
				subject := "caller"
				if scenario == "spaced subject" {
					subject = " caller "
				}
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"type": kind, "sub": subject, "owner": "owner"}).SignedString([]byte("test-key"))
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+token)
			}
			response, err := app.Test(req)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, expected, response.StatusCode)
		})
	}
}

func TestContextPolicyApplicationUsesRealSubject(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		requests <- payload
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.AuthResponse{Authorized: true})
	}))
	t.Cleanup(server.Close)
	client := auth.NewAuthClient(server.URL, true, libLog.NewNop())
	client.M2MInversionEnabled = true
	client.ForwardM2MProduct = true
	guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{AppName: "tracer", PluginAuthEnabled: true}, client)
	service := NewMockContextPolicyAdminService(gomock.NewController(t))
	service.EXPECT().GetBinding(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ model.PolicyBindingKey) (*model.PolicyBindingState, error) {
		principal, ok := contextutil.GetPrincipal(ctx)
		require.True(t, ok)
		require.Equal(t, "policy-automation", principal.ID)
		require.Equal(t, "system", principal.Type)
		return &model.PolicyBindingState{Policy: model.PolicyRevision{ID: testutil.MustDeterministicUUID(66001), Revision: 1}, Version: 1}, nil
	})
	app, _ := contextPolicyTestApp(t, service, guard)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"type": "application", "sub": "policy-automation"}).SignedString([]byte("test-key"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/v1/policy-bindings?integrationId=producer&contextId=context", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := app.Test(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	payload := <-requests
	require.Equal(t, "policy-automation", payload["sub"])
	require.Equal(t, "tracer", payload["product"])
}
