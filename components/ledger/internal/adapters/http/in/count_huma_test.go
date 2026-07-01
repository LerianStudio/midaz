// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaCountApp mounts the single transaction-count HEAD Huma operation on a
// /v1 group, mirroring the production wiring in unified-server.go: problem.Install()
// runs before huma.Register, the Huma API is built with openapi.New over a /v1
// group, an auth-shim middleware stands in for auth.Authorize("midaz","transactions",
// "head") + tenant PostAuthMiddlewares, and http.ParseUUIDPathParameters
// ("transaction") + RegisterCountTransactionRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaAssetApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses process-global
// sync.Pools — concurrent builds/requests cross-contaminate. These tests are
// sub-second; keep them sequential.
func buildHumaCountApp(t *testing.T, handler *TransactionHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","transactions","head"). A
	// rejected request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c *fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber
	// middleware (no terminal handler) before the Huma terminal on the count route.
	parse := pkgHTTP.ParseUUIDPathParameters("transaction")
	apiV1.Head("/organizations/:organization_id/ledgers/:ledger_id/transactions/metrics/count", parse)

	RegisterCountTransactionRoutes(hAPI, handler)

	return f
}

func TestHuma_CountTransactions_204WithHeader(t *testing.T) {
	// NOT parallel: buildHumaCountApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	repo := transaction.NewMockRepository(ctrl)
	repo.EXPECT().CountByFilters(gomock.Any(), orgID, ledgerID, gomock.Any()).Return(int64(7), nil).Times(1)

	handler := &TransactionHandler{Query: &query.UseCase{TransactionRepo: repo}}

	app := buildHumaCountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/metrics/count", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "7", resp.Header.Get(constant.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0 (parity with the Fiber NoContent path)")
}

func TestHuma_CountTransactions_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// No repo expectations: a rejected auth must never reach the service.
	handler := &TransactionHandler{Query: &query.UseCase{TransactionRepo: transaction.NewMockRepository(ctrl)}}

	app := buildHumaCountApp(t, handler, false)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/metrics/count", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestHuma_CountTransactions_BadStatus_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: buildCountFilter rejects an out-of-allowlist
	// status with the canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &TransactionHandler{Query: &query.UseCase{TransactionRepo: transaction.NewMockRepository(ctrl)}}

	app := buildHumaCountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/transactions/metrics/count?status=BOGUS", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// HEAD strips the response body (Fiber's app.Test drops it even on error), so
	// the canonical 400 status IS the contract — buildCountFilter rejected BOGUS
	// before the service (no repo expectation set proves the service was unreached).
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad status stays canonical 400 — no native Huma 422")
}

func TestHuma_CountTransactions_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.New()
	ledgerID := uuid.New()

	// Service must never be reached: ParseUUIDPathParameters rejects the bad
	// ledger id with the canonical 0065 / 400 before Huma.
	handler := &TransactionHandler{Query: &query.UseCase{TransactionRepo: transaction.NewMockRepository(ctrl)}}

	app := buildHumaCountApp(t, handler, true)

	_ = ledgerID
	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/not-a-uuid/transactions/metrics/count", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// HEAD strips the body; the canonical 400 status is the contract.
	// ParseUUIDPathParameters rejected the bad ledger id before Huma (no repo
	// expectation set proves the service was unreached).
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")
}
