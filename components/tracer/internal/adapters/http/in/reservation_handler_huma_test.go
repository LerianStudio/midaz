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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// reservationSpyService is a ReservationService stub that records the tenant ID it
// sees on its incoming ctx AND which action was invoked. It is the ctx-threading
// probe (tenant from c.UserContext() must reach the service via the Huma handler
// ctx with no bridge) AND the shell-wiring probe: capturedAction proves each of the
// four lifecycle shells (ConfirmHuma/ReleaseHuma/ConfirmByTransactionHuma/
// ReleaseByTransactionHuma) delegates to the right service method with the right
// terminal status.
type reservationSpyService struct {
	capturedTenant string
	capturedAction string
	capturedTxID   uuid.UUID
	capturedResID  uuid.UUID

	reserveResult *services.ReserveResult
	reserveErr    error
	confirmErr    error
	releaseErr    error
	byTxFlipped   int
	byTxErr       error
}

func (s *reservationSpyService) Reserve(ctx context.Context, transactionID uuid.UUID, _ *model.CheckLimitsInput, _ bool) (*services.ReserveResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.capturedTxID = transactionID
	return s.reserveResult, s.reserveErr
}

func (s *reservationSpyService) Confirm(ctx context.Context, reservationID uuid.UUID) error {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.capturedAction = "Confirm"
	s.capturedResID = reservationID
	return s.confirmErr
}

func (s *reservationSpyService) Release(ctx context.Context, reservationID uuid.UUID) error {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.capturedAction = "Release"
	s.capturedResID = reservationID
	return s.releaseErr
}

func (s *reservationSpyService) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.capturedAction = "ConfirmByTransaction"
	s.capturedTxID = transactionID
	return s.byTxFlipped, s.byTxErr
}

func (s *reservationSpyService) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (int, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.capturedAction = "ReleaseByTransaction"
	s.capturedTxID = transactionID
	return s.byTxFlipped, s.byTxErr
}

// buildHumaReservationApp mirrors buildHumaRuleApp for the five reservation ops. See
// buildHumaRuleApp's header for the full production-wiring rationale. The handler
// gets a FIXED clock so NormalizeAndReserveValidate's timestamp window is
// deterministic against testutil.FixedTime().
//
// MUST-NOT-PARALLELIZE (like every 2b copy): tests built through this helper CANNOT
// call t.Parallel(). libProblem.Install() swaps the process-global huma.NewError
// hook, and Huma validation uses process-global sync.Pools; concurrent
// builds/requests cross-contaminate. -race does NOT catch it (the bug is logical,
// not a data race). These tests are sub-second — keep them sequential.
func buildHumaReservationApp(t *testing.T, svc ReservationService, tenantID string) *fiber.App {
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

	h, err := NewReservationHandler(svc, clock.NewFixedClock(testutil.FixedTime()))
	require.NoError(t, err)
	RegisterReservationRoutes(hAPI, h)

	return f
}

// validReserveBody builds a reserve body that passes NormalizeAndReserveValidate
// against the fixed test clock (transactionTimestamp == FixedTime()).
func validReserveBody(t *testing.T) []byte {
	t.Helper()

	body, err := json.Marshal(newValidReserveRequest())
	require.NoError(t, err)

	return body
}

func TestHuma_Reserve_Success(t *testing.T) {
	// NOT parallel: buildHumaReservationApp mutates process-global huma state
	// (libProblem.Install + Huma validation pools). See buildHumaRuleApp.
	reservationID := testutil.MustDeterministicUUID(10)
	svc := &reservationSpyService{reserveResult: &services.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
	app := buildHumaReservationApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader(validReserveBody(t)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Reserve must return 201 through Huma")
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")

	var got ReserveResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ReserveResponse: %s", string(respBody))
	assert.False(t, got.Denied)
	require.Len(t, got.ReservationIDs, 1)
	assert.Equal(t, reservationID, got.ReservationIDs[0])
	assert.Equal(t, testutil.MustDeterministicUUID(1), got.TransactionID)

	assert.Equal(t, "tenant-alpha", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

// TestHuma_Reserve_PayloadTooLarge pins the payload-size contract: a body over
// maxPayloadSize (100KB) must reach the core's imperative size check and produce the
// canonical ErrPayloadTooLarge (code 0143) — NOT a native Huma 422/413. Huma has no
// Fiber-style body limit, so the check stays imperative in the core.
func TestHuma_Reserve_PayloadTooLarge(t *testing.T) {
	svc := &reservationSpyService{}
	app := buildHumaReservationApp(t, svc, "tenant-gamma")

	big := `{"filler":"` + strings.Repeat("x", 100*1024+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader([]byte(big)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"),
		"error must carry the RFC 9457 media type")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0143", got["code"], "code must be ErrPayloadTooLarge, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on an oversized payload")
}

// TestHuma_Reserve_MalformedJSON pins the SkipValidateBody + RawBody contract: a
// syntactically broken JSON body must reach the core's imperative json.Unmarshal and
// produce the canonical 400 / code 0094 (ErrInvalidRequestBody), NOT a native Huma
// body-schema rejection.
func TestHuma_Reserve_MalformedJSON(t *testing.T) {
	svc := &reservationSpyService{}
	app := buildHumaReservationApp(t, svc, "tenant-zeta")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"malformed JSON must be the canonical 400 — no native Huma body validation")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0094", got["code"], "code must be ErrInvalidRequestBody, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on malformed JSON")
}

// TestHuma_Confirm_Success asserts ConfirmHuma delegates to service.Confirm and
// returns 200 with the CONFIRMED terminal status keyed on the reservation id.
func TestHuma_Confirm_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(20)
	svc := &reservationSpyService{}
	app := buildHumaReservationApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/"+id.String()+"/confirm", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Confirm must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got ReservationActionResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ReservationActionResponse: %s", string(respBody))
	assert.Equal(t, id, got.ReservationID)
	assert.Equal(t, string(model.StatusConfirmed), got.Status)

	assert.Equal(t, "Confirm", svc.capturedAction, "ConfirmHuma must call service.Confirm")
	assert.Equal(t, id, svc.capturedResID)
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

// TestHuma_Release_Success asserts ReleaseHuma delegates to service.Release and
// returns 200 with the RELEASED terminal status.
func TestHuma_Release_Success(t *testing.T) {
	id := testutil.MustDeterministicUUID(21)
	svc := &reservationSpyService{}
	app := buildHumaReservationApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/"+id.String()+"/release", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Release must return 200 through Huma")

	var got ReservationActionResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ReservationActionResponse: %s", string(respBody))
	assert.Equal(t, id, got.ReservationID)
	assert.Equal(t, string(model.StatusReleased), got.Status)

	assert.Equal(t, "Release", svc.capturedAction, "ReleaseHuma must call service.Release")
	assert.Equal(t, "tenant-beta", svc.capturedTenant)
}

// TestHuma_ConfirmByTransaction_Success asserts ConfirmByTransactionHuma reads the
// transaction_id path param, delegates to service.ConfirmByTransaction, and returns
// 200 with CONFIRMED + the flipped count.
func TestHuma_ConfirmByTransaction_Success(t *testing.T) {
	txID := testutil.MustDeterministicUUID(30)
	svc := &reservationSpyService{byTxFlipped: 2}
	app := buildHumaReservationApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/transaction/"+txID.String()+"/confirm", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ConfirmByTransaction must return 200 through Huma")

	var got TransactionActionResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be TransactionActionResponse: %s", string(respBody))
	assert.Equal(t, txID, got.TransactionID)
	assert.Equal(t, string(model.StatusConfirmed), got.Status)
	assert.Equal(t, 2, got.Flipped)

	assert.Equal(t, "ConfirmByTransaction", svc.capturedAction, "ConfirmByTransactionHuma must call service.ConfirmByTransaction")
	assert.Equal(t, txID, svc.capturedTxID, "transaction_id path param must resolve to the service call")
	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
}

// TestHuma_ReleaseByTransaction_Success asserts ReleaseByTransactionHuma reads the
// transaction_id path param, delegates to service.ReleaseByTransaction, and returns
// 200 with RELEASED + the flipped count.
func TestHuma_ReleaseByTransaction_Success(t *testing.T) {
	txID := testutil.MustDeterministicUUID(31)
	svc := &reservationSpyService{byTxFlipped: 0} // idempotent no-op: flipped=0 is a valid 200
	app := buildHumaReservationApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/transaction/"+txID.String()+"/release", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ReleaseByTransaction must return 200 through Huma")

	var got TransactionActionResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be TransactionActionResponse: %s", string(respBody))
	assert.Equal(t, txID, got.TransactionID)
	assert.Equal(t, string(model.StatusReleased), got.Status)
	assert.Equal(t, 0, got.Flipped)

	assert.Equal(t, "ReleaseByTransaction", svc.capturedAction, "ReleaseByTransactionHuma must call service.ReleaseByTransaction")
	assert.Equal(t, txID, svc.capturedTxID)
	assert.Equal(t, "tenant-beta", svc.capturedTenant)
}

// TestHuma_Confirm_BadUUID pins the malformed reservation-id path-param contract: a
// non-UUID {id} must reach the core's imperative uuid.Parse and produce the
// canonical 400 / code 0065 (ErrInvalidPathParameter) / entityType Reservation —
// NOT a native Huma 422. Path params carry NO format tag.
func TestHuma_Confirm_BadUUID(t *testing.T) {
	svc := &reservationSpyService{}
	app := buildHumaReservationApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/not-a-uuid/confirm", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"], "code must be ErrInvalidPathParameter, identical to the Fiber path")
	assert.Equal(t, "Reservation", got["entityType"], "canonical entityType")
	assert.Empty(t, svc.capturedAction, "service must not be reached on a bad path param")
}

// TestHuma_ConfirmByTransaction_BadUUID pins the malformed transaction_id path-param
// contract: a non-UUID {transaction_id} must reach uuid.Parse and produce the
// canonical 400 / code 0065 with the transaction_id field annotation — NOT a native
// Huma 422.
func TestHuma_ConfirmByTransaction_BadUUID(t *testing.T) {
	svc := &reservationSpyService{}
	app := buildHumaReservationApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/transaction/not-a-uuid/confirm", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"])
	assert.Empty(t, svc.capturedAction, "service must not be reached on a bad path param")
}

// TestHuma_Confirm_NotFound pins the not-found error contract: a confirm against a
// missing reservation must reach classifyReservationServiceError and produce the
// canonical 404 / code 0482 (ErrReservationNotFound) — proving the shared
// classification flows through humaProblem identically to the Fiber path.
func TestHuma_Confirm_NotFound(t *testing.T) {
	id := testutil.MustDeterministicUUID(22)
	svc := &reservationSpyService{confirmErr: constant.ErrReservationNotFound}
	app := buildHumaReservationApp(t, svc, "tenant-delta")

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations/"+id.String()+"/confirm", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "missing reservation must be the canonical 404")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0482", got["code"], "code must be ErrReservationNotFound, identical to the Fiber path")
}
