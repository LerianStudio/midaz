// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaEncryptionApp mounts the two encryption Huma operations on a /v2 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v2
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

	return buildHumaEncryptionAppWithTenant(t, handler, authOK, "")
}

// buildHumaEncryptionAppWithTenant is buildHumaEncryptionApp with an explicit tenant
// id stamped onto the request context, standing in for the tenant PostAuthMiddlewares.
// An empty tenantID leaves the context untouched, which is the single-tenant posture
// where no tenant middleware runs.
func buildHumaEncryptionAppWithTenant(t *testing.T, handler *EncryptionHandler, authOK bool, tenantID string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV2 := f.Group("/v2")

	apiV2.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		if tenantID != "" {
			c.SetContext(tmcore.ContextWithTenantID(c.Context(), tenantID))
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	parse := pkgHTTP.ParseUUIDPathParameters("organization")
	apiV2.Post("/organizations/:organization_id/encryption/provision", parse)
	apiV2.Get("/organizations/:organization_id/encryption/status", parse)

	RegisterEncryptionRoutes(hAPI, handler, v2OpSuffix)

	return f
}

func TestProvisionEncryption_Success(t *testing.T) {
	// NOT parallel: buildHumaEncryptionApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
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
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestProvisionEncryption_ValidationRejectedByCore(t *testing.T) {
	// NOT parallel: process-global huma state. A missing actor must be rejected by
	// the shared DecodeAndValidate/core pipeline (canonical 400), NOT a native
	// Huma 422 — proving SkipValidateBody keeps validation in the core.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// No Provision expectation: validation must fail before the service is called.
	mockService := encryption.NewMockProvisioningService(ctrl)
	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestProvisionEncryption_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// No expectations: a rejected auth must never reach the service.
	mockService := encryption.NewMockProvisioningService(ctrl)
	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetProvisioningStatus_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	status := mmodel.RegistryStatusActive
	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		GetProvisioningStatus(gomock.Any(), orgID.String()).
		Return(&status, nil).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/encryption/status", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
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

func TestProvisionEncryption_MissingReasonRejectedByCore(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// No Provision expectation: validation must fail before the service is called.
	mockService := encryption.NewMockProvisioningService(ctrl)
	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestProvisionEncryption_AlreadyProvisioned(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		Provision(gomock.Any(), gomock.Any()).
		Return(encryption.ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrRegistryAlreadyExists, encryption.EntityOrganizationEncryption)).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrRegistryAlreadyExists.Error(), got["code"])
}

func TestProvisionEncryption_ServiceFailure(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		Provision(gomock.Any(), gomock.Any()).
		Return(encryption.ProvisionResult{}, pkg.ValidateBusinessError(constant.ErrOrganizationEncryptionFailed, encryption.EntityOrganizationEncryption)).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrOrganizationEncryptionFailed.Error(), got["code"])
}

func TestProvisionEncryption_ReservedTenantIDRejected(t *testing.T) {
	// NOT parallel: process-global huma state. Multi-tenant mode where the tenant
	// middleware stamped a real tenant literally named "default": it collides with
	// the single-tenant flat-base sentinel and must be rejected before the service.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	// No expectations: rejection happens before the service call.
	mockService := encryption.NewMockProvisioningService(ctrl)
	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionAppWithTenant(t, handler, true, "default")

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, constant.ErrReservedTenantID.Error(), got["code"],
		"reserved tenant id rejection should map to ErrReservedTenantID code")
}

func TestProvisionEncryption_SingleTenantSentinelForwarded(t *testing.T) {
	// NOT parallel: process-global huma state. No tenant middleware ran, so the
	// context carries no tenant id and the core substitutes the "default"
	// flat-base sentinel.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		Provision(gomock.Any(), gomock.Cond(func(x any) bool {
			req, ok := x.(encryption.ProvisionInput)
			return ok && req.OrganizationID == orgID.String() && req.TenantID == "default"
		})).
		Return(encryption.ProvisionResult{
			OrganizationID: orgID.String(),
			KEKPath:        "transit/keys/org-" + orgID.String(),
			RegistryStatus: mmodel.RegistryStatusActive,
		}, nil).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestProvisionEncryption_ResponseCarriesNoSecretField(t *testing.T) {
	// NOT parallel: process-global huma state.
	//
	// Forward-guard on response SHAPE, not a RED-capable leak test. ProvisionResult
	// (and therefore ProvisionEncryptionResponse) has no field that can carry raw
	// legacy secret material: only org id, KEK path, the two primary key IDs and
	// status are mapped. This case guards against a FUTURE field addition that
	// would serialize secret-derived data. The sentinels stand in for imported
	// LCRYPTO_ENCRYPT_SECRET_KEY / LCRYPTO_HASH_SECRET_KEY material.
	const (
		sentinelLegacyAESHex     = "not-a-real-aes-key-test-marker-aaaaaaaaaaaaaaaaaa"
		sentinelLegacyHMACSecret = "not-a-real-hmac-test-marker-do-not-leak"
	)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		Provision(gomock.Any(), gomock.Any()).
		Return(encryption.ProvisionResult{
			OrganizationID:   orgID.String(),
			KEKPath:          "transit/keys/org-" + orgID.String(),
			AEADPrimaryKeyID: 123456,
			PRFPrimaryKeyID:  789012,
			RegistryStatus:   mmodel.RegistryStatusActive,
		}, nil).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"actor": "admin@example.com", "reason": "Initial encryption setup"})
	req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+"/encryption/provision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	rendered := string(respBody)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotContains(t, rendered, sentinelLegacyAESHex,
		"provision response must never contain raw legacy AES key material")
	assert.NotContains(t, rendered, sentinelLegacyHMACSecret,
		"provision response must never contain raw legacy HMAC secret")

	var got mmodel.ProvisionEncryptionResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", rendered)
	assert.NotEmpty(t, got.OrganizationID)
	assert.NotEmpty(t, got.KEKPath)
	assert.Equal(t, string(mmodel.RegistryStatusActive), got.Status)
}

func TestGetProvisioningStatus_NotProvisioned(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		GetProvisioningStatus(gomock.Any(), orgID.String()).
		Return(nil, nil).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/encryption/status", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got mmodel.ProvisioningStatusResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, orgID.String(), got.OrganizationID)
	assert.Empty(t, got.Status, "status should be empty for a not-provisioned organization")
	assert.False(t, got.Provisioned)
}

func TestGetProvisioningStatus_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		GetProvisioningStatus(gomock.Any(), orgID.String()).
		Return(nil, errors.New("database error")).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/encryption/status", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetProvisioningStatus_ContextCancelled(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())

	mockService := encryption.NewMockProvisioningService(ctrl)
	mockService.EXPECT().
		GetProvisioningStatus(gomock.Any(), orgID.String()).
		Return(nil, context.Canceled).
		Times(1)

	handler := &EncryptionHandler{ProvisioningService: mockService}
	app := buildHumaEncryptionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v2/organizations/"+orgID.String()+"/encryption/status", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
