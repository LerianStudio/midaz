// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func contextPolicyTestApp(t *testing.T, service ContextPolicyAdminService, guard *middleware.AuthGuard) (*fiber.App, huma.API) {
	t.Helper()
	problem.Install()
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	api := app.Group("/v1", func(c fiber.Ctx) error {
		c.SetContext(tmcore.ContextWithTenantID(c.Context(), "verified-tenant"))
		return c.Next()
	})
	ha := openapi.New(app, api, openapi.Config{Title: "policy test", Version: "1", Servers: []string{"/v1"}})
	pkgHTTP.InstallSchemaNamer(ha)
	h, err := NewContextPolicyHandler(service, 10, 65536)
	require.NoError(t, err)
	if guard == nil {
		RegisterContextPolicyRoutes(ha, h)
	} else {
		registerTracerHumaRoutes(api, ha, tracerHumaHandlers{Guard: guard, Rule: &Handler{}, Limit: &LimitHandler{}, Validation: &ValidationHandler{}, TransactionValidation: &TransactionValidationHandler{}, AuditEvent: &AuditEventHandler{}, ContextPolicy: h})
	}
	return app, ha
}

func policyHTTPDocument() map[string]any {
	return map[string]any{"id": testutil.MustDeterministicUUID(66001), "revision": int64(1), "defaultDecision": "DENY", "rules": []any{map[string]any{"id": testutil.MustDeterministicUUID(66002), "revision": int64(1), "expression": "true", "action": "REVIEW"}}}
}

func TestContextPolicyPublishHTTP(t *testing.T) {
	service := NewMockContextPolicyAdminService(gomock.NewController(t))
	app, api := contextPolicyTestApp(t, service, nil)
	service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, p model.ContextPolicy) error {
		require.Equal(t, "verified-tenant", tmcore.GetTenantIDContext(ctx))
		require.Equal(t, model.DecisionDeny, p.DefaultDecision)
		require.Len(t, p.Rules, 1)
		require.Equal(t, model.DecisionReview, p.Rules[0].Action)
		return nil
	})
	body, err := json.Marshal(policyHTTPDocument())
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/policies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response, err := app.Test(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var result map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.Equal(t, "DENY", result["defaultDecision"])
	op := api.OpenAPI().Paths["/policies"].Post
	require.Equal(t, []map[string][]string{{"BearerAuth": {}}}, op.Security)
	require.NotEmpty(t, op.RequestBody.Content["application/json"].Schema.Ref)
}

func TestContextPolicyRejectsInvalidBodies(t *testing.T) {
	for _, scenario := range []string{"missing rules", "null rules", "tenant", "missing default", "invalid default", "duplicate rule", "zero revision", "unknown rule field", "trailing body", "overflow", "null"} {
		t.Run(scenario, func(t *testing.T) {
			service := NewMockContextPolicyAdminService(gomock.NewController(t))
			app, _ := contextPolicyTestApp(t, service, nil)
			input := policyHTTPDocument()
			switch scenario {
			case "missing rules":
				delete(input, "rules")
			case "null rules":
				input["rules"] = nil
			case "tenant":
				input["tenantId"] = "forged"
			case "missing default":
				delete(input, "defaultDecision")
			case "invalid default":
				input["defaultDecision"] = "REVIEW"
			case "duplicate rule":
				rules := input["rules"].([]any)
				input["rules"] = append(rules, rules[0])
			case "zero revision":
				input["revision"] = 0
			case "unknown rule field":
				input["rules"].([]any)[0].(map[string]any)["tenantId"] = "forged"
			}
			body, err := json.Marshal(input)
			require.NoError(t, err)
			if scenario == "trailing body" {
				body = append(body, []byte(" {}")...)
			}
			if scenario == "overflow" {
				body = bytes.Repeat([]byte("x"), 65537)
			}
			if scenario == "null" {
				body = []byte("null")
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/policies", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response, err := app.Test(req)
			require.NoError(t, err)
			defer response.Body.Close()
			if scenario == "overflow" {
				require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
			} else {
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
			}
		})
	}
}

func TestContextPolicyBindHTTP(t *testing.T) {
	for _, failure := range []error{nil, constant.ErrContextPolicyConflict, constant.ErrContextPolicyUnavailable} {
		service := NewMockContextPolicyAdminService(gomock.NewController(t))
		app, _ := contextPolicyTestApp(t, service, nil)
		key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "ledger-context"}
		ref := model.PolicyRevision{ID: testutil.MustDeterministicUUID(66001), Revision: 2}
		expected := int64(7)
		service.EXPECT().Bind(gomock.Any(), key, ref, &expected).Return(&model.PolicyBindingState{Policy: ref, Version: 8}, failure)
		body, err := json.Marshal(map[string]any{"integrationId": key.IntegrationID, "contextId": key.ContextID, "policyId": ref.ID, "policyRevision": ref.Revision, "expectedVersion": expected})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPut, "/v1/policy-bindings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response, err := app.Test(req)
		require.NoError(t, err)
		content, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		want := http.StatusOK
		if failure == constant.ErrContextPolicyConflict {
			want = http.StatusConflict
		}
		if failure == constant.ErrContextPolicyUnavailable {
			want = http.StatusServiceUnavailable
		}
		require.Equal(t, want, response.StatusCode, string(content))
		if failure != nil {
			require.Contains(t, string(content), failure.Error())
		} else {
			require.Contains(t, string(content), `"version":8`)
		}
	}
}

func TestContextPolicyReadHTTP(t *testing.T) {
	service := NewMockContextPolicyAdminService(gomock.NewController(t))
	app, _ := contextPolicyTestApp(t, service, nil)
	id := testutil.MustDeterministicUUID(66001)
	ref := model.PolicyRevision{ID: id, Revision: 2}
	service.EXPECT().GetRevision(gomock.Any(), ref).Return(&model.ContextPolicy{ID: id, Revision: 2, DefaultDecision: model.DecisionAllow}, nil)
	key := model.PolicyBindingKey{IntegrationID: "producer", ContextID: "opaque-context"}
	service.EXPECT().GetBinding(gomock.Any(), key).Return(&model.PolicyBindingState{Policy: ref, Version: 4}, nil)
	for _, path := range []string{"/v1/policies/" + id.String() + "/revisions/2", "/v1/policy-bindings?integrationId=producer&contextId=opaque-context"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	for _, path := range []string{"/v1/policies/not-uuid/revisions/2", "/v1/policies/" + id.String() + "/revisions/0", "/v1/policy-bindings?integrationId=producer"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
}

func TestContextPolicyDisabledRoutes(t *testing.T) {
	deps := newTestRouterDeps(t, middleware.AuthGuardConfig{})
	app := deps.build()
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/policy-bindings?integrationId=producer&contextId=context", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestContextPolicyWrappedUnavailable(t *testing.T) {
	service := NewMockContextPolicyAdminService(gomock.NewController(t))
	app, _ := contextPolicyTestApp(t, service, nil)
	service.EXPECT().GetBinding(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("private storage details: %w", constant.ErrContextPolicyUnavailable))
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/v1/policy-bindings?integrationId=p&contextId=c", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(content), "0518")
	require.NotContains(t, string(content), "private storage details")
}
