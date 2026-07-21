// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	tmctx "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// validationSpyService is a ValidationService stub that records the tenant ID it
// sees on its incoming ctx. It is the ctx-threading probe: the tenant middleware
// writes the tenant into c.UserContext(), the humafiber v2 adapter builds the
// Huma handler ctx from c.UserContext(), and the handler passes that ctx to the
// service — a non-empty capturedTenant proves the whole chain end to end without
// a bridge.
type validationSpyService struct {
	capturedTenant string
	result         *services.ValidateResult
	err            error
}

func (s *validationSpyService) Validate(ctx context.Context, _ *model.ValidationRequest) (*services.ValidateResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.result, s.err
}

// buildHumaValidationApp mirrors buildHumaRuleApp for the single Validate op. See
// buildHumaRuleApp's header for the full production-wiring rationale.
//
// MUST-NOT-PARALLELIZE (like every 2b copy): tests built through this helper
// CANNOT call t.Parallel(). libProblem.Install() swaps the process-global
// huma.NewError hook, and Huma validation uses process-global sync.Pools;
// concurrent builds/requests cross-contaminate. -race does NOT catch it (the bug
// is logical, not a data race). These tests are sub-second — keep them
// sequential.
func buildHumaValidationApp(t *testing.T, svc ValidationService, tenantID string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	api := f.Group("/v1")
	api.Use(func(c *fiber.Ctx) error {
		if tenantID != "" {
			c.SetUserContext(tmctx.ContextWithTenantID(c.UserContext(), tenantID))
		}
		return c.Next()
	})

	hAPI := openapi.New(f, api, openapi.Config{Title: "tracer-test", Version: "test", Servers: []string{"/v1"}})

	// Fixed clock so NormalizeAndValidate's timestamp check is deterministic.
	h, err := NewValidationHandler(svc, clock.NewFixedClock(testutil.FixedTime()))
	require.NoError(t, err)
	RegisterValidationRoutes(hAPI, h)

	return f
}

// validValidationRequestBody builds a request body that passes NormalizeAndValidate
// against the fixed test clock (transactionTimestamp == FixedTime()).
func validValidationRequestBody(t *testing.T) []byte {
	t.Helper()

	body, err := json.Marshal(model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: testutil.FixedTime(),
		Account:              model.AccountContext{ID: testutil.MustDeterministicUUID(2)},
	})
	require.NoError(t, err)

	return body
}

// TestHuma_Validate_NewReturns201 pins the dual-status contract: a NEW (non-
// duplicate) validation returns 201, driven by the ValidateOutputHuma.Status
// field the shell sets from result.IsDuplicate — NOT a fixed DefaultStatus.
func TestHuma_Validate_NewReturns201(t *testing.T) {
	// NOT parallel: buildHumaValidationApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
	resp := &model.ValidationResponse{
		ValidationID: testutil.MustDeterministicUUID(10),
		RequestID:    testutil.MustDeterministicUUID(1),
		EvaluationResult: model.EvaluationResult{
			Decision:       model.DecisionAllow,
			MatchedRuleIDs: []uuid.UUID{},
			Reason:         "No matching rules found",
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  15,
	}
	svc := &validationSpyService{result: &services.ValidateResult{Response: resp, IsDuplicate: false}}
	app := buildHumaValidationApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader(validValidationRequestBody(t)))
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, httpResp.StatusCode, "new validation must return 201 through Huma")
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	// validationId is the model.ValidationResponse json tag — proves body is verbatim.
	assert.Equal(t, testutil.MustDeterministicUUID(10).String(), got["validationId"])
	assert.Equal(t, testutil.MustDeterministicUUID(1).String(), got["requestId"])
	assert.Equal(t, "ALLOW", got["decision"])

	assert.Equal(t, "tenant-alpha", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

// TestHuma_Validate_DuplicateReturns200 pins the other half of the dual-status
// contract: a DUPLICATE (idempotent) validation returns 200 — same body shape,
// different status, from the same Status field.
func TestHuma_Validate_DuplicateReturns200(t *testing.T) {
	resp := &model.ValidationResponse{
		ValidationID: testutil.MustDeterministicUUID(10),
		RequestID:    testutil.MustDeterministicUUID(1),
		EvaluationResult: model.EvaluationResult{
			Decision:       model.DecisionAllow,
			MatchedRuleIDs: []uuid.UUID{},
			Reason:         "No matching rules found",
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  15,
	}
	svc := &validationSpyService{result: &services.ValidateResult{Response: resp, IsDuplicate: true}}
	app := buildHumaValidationApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader(validValidationRequestBody(t)))
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, httpResp.StatusCode, "duplicate validation must return 200 through Huma")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, testutil.MustDeterministicUUID(10).String(), got["validationId"])
	assert.Equal(t, "tenant-beta", svc.capturedTenant)
}

// TestHuma_Validate_PayloadTooLarge pins the payload-size contract: a body over
// maxPayloadSize (100KB) must reach the core's imperative size check and produce
// the canonical ErrPayloadTooLarge (code 0143) — NOT a native Huma 422/413 and
// NOT a body-schema rejection. Huma has no Fiber-style body limit, so the check
// stays imperative in the core.
func TestHuma_Validate_PayloadTooLarge(t *testing.T) {
	svc := &validationSpyService{}
	app := buildHumaValidationApp(t, svc, "tenant-gamma")

	// A syntactically valid JSON body larger than 100KB.
	big := `{"filler":"` + strings.Repeat("x", 100*1024+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader([]byte(big)))
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	require.NoError(t, err)

	assert.Equal(t, "application/problem+json", httpResp.Header.Get("Content-Type"),
		"error must carry the RFC 9457 media type")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0143", got["code"], "code must be ErrPayloadTooLarge, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on an oversized payload")
}

// TestHuma_Validate_MalformedJSON pins the SkipValidateBody + RawBody contract: a
// syntactically broken JSON body must reach the core's imperative json.Unmarshal
// and produce the canonical 400 / code 0094 (ErrInvalidRequestBody), NOT a native
// Huma body-schema rejection.
func TestHuma_Validate_MalformedJSON(t *testing.T) {
	svc := &validationSpyService{}
	app := buildHumaValidationApp(t, svc, "tenant-zeta")

	req := httptest.NewRequest(http.MethodPost, "/v1/validations", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, httpResp.StatusCode,
		"malformed JSON must be the canonical 400 — no native Huma body validation")
	assert.Equal(t, "application/problem+json", httpResp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0094", got["code"], "code must be ErrInvalidRequestBody, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on malformed JSON")
}
