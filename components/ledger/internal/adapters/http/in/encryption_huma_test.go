// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaEncryptionApp mounts the two encryption Huma operations on a /v1 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v1
// group, an auth-shim middleware stands in for auth.Authorize("midaz","encryption",
// verb) + tenant PostAuthMiddlewares, and http.ParseUUIDPathParameters("organization")
// + RegisterEncryptionRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as the asset exemplar's buildHumaAssetApp):
// libProblem.Install() swaps the process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools — concurrent builds/requests
// cross-contaminate. These tests are sub-second; keep them sequential.
func buildHumaEncryptionApp(t *testing.T, handler *EncryptionHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	parse := pkgHTTP.ParseUUIDPathParameters("organization")
	apiV1.Post("/organizations/:organization_id/encryption/provision", parse)
	apiV1.Get("/organizations/:organization_id/encryption/status", parse)

	RegisterEncryptionRoutes(hAPI, handler)

	return f
}

func TestHuma_ProvisionEncryption_Success(t *testing.T) {
	// NOT parallel: buildHumaEncryptionApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	mockService := NewMockProvisioningService(ctrl)
	// Single-tenant harness: no tenant middleware runs, so the core resolves the
	// reserved "default" flat-base sentinel (no error) and passes it through.
	mockService.EXPECT().
		Provision(gomock.Any(), gomock.Cond(func(x any) bool {
			req, ok := x.(encryption.ProvisionInput)
			return ok && req.OrganizationID == orgID.String() &&
				req.Actor == "admin@example.com" && req.Reason == "Initial encryption setup"
		})).
		Return(encryption.ProvisionResult{
			OrganizationID:   orgID.String(),
			KEKPath:          "transit/keys/org-" + orgID.String(),
			AEADPrimaryKeyID: 1,
			PRFPrimaryKeyID:  1,
			RegistryStatus:   mmodel.RegistryStatusActive,
		}, nil).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got mmodel.ProvisionEncryptionResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, orgID.String(), got.OrganizationID)
	assert.Equal(t, string(mmodel.RegistryStatusActive), got.Status)
}

func TestHuma_ProvisionEncryption_ValidationRejectedByCore(t *testing.T) {
	// NOT parallel: process-global huma state. A missing actor must be rejected by
	// the shared DecodeAndValidate/core pipeline (canonical 400), NOT a native
	// Huma 422 — proving SkipValidateBody keeps validation in the core.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// No Provision expectation: validation must fail before the service is called.
	mockService := NewMockProvisioningService(ctrl)
	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHuma_ProvisionEncryption_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	// No expectations: a rejected auth must never reach the service.
	mockService := NewMockProvisioningService(ctrl)
	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHuma_GetProvisioningStatus_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()

	status := mmodel.RegistryStatusActive
	mockService := NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		GetProvisioningStatus(gomock.Any(), orgID.String()).
		Return(&status, nil).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/encryption/status", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got mmodel.ProvisioningStatusResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, orgID.String(), got.OrganizationID)
	assert.Equal(t, string(mmodel.RegistryStatusActive), got.Status)
	assert.True(t, got.Provisioned)
}
