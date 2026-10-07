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
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// stubAccountCreator satisfies composition.AccountCreator for handler tests.
type stubAccountCreator struct {
	account *mmodel.Account
	err     error
}

func (s stubAccountCreator) CreateAccount(_ context.Context, _, _ uuid.UUID, _ *mmodel.CreateAccountInput, _ string, _ command.RouteHolderPolicy) (*mmodel.Account, error) {
	return s.account, s.err
}

// stubInstrumentCreator satisfies composition.InstrumentCreator for handler tests.
type stubInstrumentCreator struct {
	instrument *mmodel.Instrument
	err        error
}

func (s stubInstrumentCreator) CreateInstrument(_ context.Context, _ string, _ uuid.UUID, _ *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	return s.instrument, s.err
}

// buildHumaCompositionApp mounts the single composition Huma operation on a /v2
// group, faithfully mirroring the production wiring in unified-server.go:
// problem.Install() runs before any huma.Register, the Huma API is built with
// openapi.New over a /v2 group, an auth-shim middleware stands in for
// auth.Authorize("midaz","accounts","post") + tenant PostAuthMiddlewares, and
// http.ParseUUIDPathParameters("holder") + RegisterCompositionRoutes attach the
// chain. See asset_huma_test.go's buildHumaAssetApp for the full rationale.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global
// huma.NewError hook and Huma validation uses process-global sync.Pools —
// concurrent builds/requests cross-contaminate. These tests are sub-second; keep
// them sequential.
func buildHumaCompositionApp(t *testing.T, handler *CompositionHandler, authOK bool) *fiber.App {
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

		return c.Next()
	})

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	// The :id path param is the holder; ParseUUIDPathParameters("holder") validates
	// it (mirrors composition_routes.go). Registered group-relative on apiV2.
	parse := pkgHTTP.ParseUUIDPathParameters("holder")
	apiV2.Post("/organizations/:organization_id/ledgers/:ledger_id/holders/:holder_id/accounts", parse)

	RegisterCompositionRoutes(hAPI, handler, v2OpSuffix)

	return f
}

func compositionURL(orgID, ledgerID, holderID uuid.UUID) string {
	return "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() +
		"/holders/" + holderID.String() + "/accounts"
}

func validCompositionBody() []byte {
	body, _ := json.Marshal(map[string]any{
		"assetCode": "USD",
		"type":      "deposit",
	})

	return body
}

func TestCreateHolderAccount_Success(t *testing.T) {
	// NOT parallel: buildHumaCompositionApp mutates process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	createdAccount := &mmodel.Account{ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), AssetCode: "USD", Type: "deposit"}

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{account: createdAccount},
		stubInstrumentCreator{},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")
	assert.NotContains(t, string(respBody), "$ref")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.Contains(t, got, "account")
	acc, ok := got["account"].(map[string]any)
	require.True(t, ok, "account object present")
	assert.Equal(t, createdAccount.ID, acc["id"])
}

func TestCreateHolderAccount_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: a rejected auth returns the ledger 401.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, false)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateHolderAccount_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	// Missing required assetCode/type -> imperative ValidateStruct -> canonical 400,
	// service never reached.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "no asset code"})
	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "imperative validation stays 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.NotEmpty(t, got["code"], "canonical code present")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestCreateHolderAccount_MalformedBody_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed body stays 400 — no 500, no native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidRequestBody.Error(), got["code"], "malformed-body code preserved (0094)")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"])
}

func TestCreateHolderAccount_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad holder
	// id with the canonical 0065 / 400 before Huma.
	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	app := buildHumaCompositionApp(t, handler, true)

	url := "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/holders/not-a-uuid/accounts"
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidPathParameter.Error(), got["code"])
}

func TestCreateHolderAccount_BusinessError_Preserved(t *testing.T) {
	// NOT parallel: process-global huma state. The account-create fails with a
	// business error; HumaProblem must project the canonical envelope verbatim.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	bizErr := pkg.ValidateBusinessError(cn.ErrAssetCodeNotFound, "Account")

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{err: bizErr},
		stubInstrumentCreator{},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(validCompositionBody()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAssetCodeNotFound.Error(), got["code"], "business error code preserved across Huma")
}

func TestCreateHolderAccount_WithInstrument_201(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	createdAccount := &mmodel.Account{ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), AssetCode: "USD", Type: "deposit"}
	instrumentID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{account: createdAccount},
		stubInstrumentCreator{instrument: &mmodel.Instrument{ID: &instrumentID}},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"assetCode":      "USD",
		"type":           "deposit",
		"bankingDetails": map[string]any{},
	})

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var got mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.NotNil(t, got.Account)
	require.NotNil(t, got.Instrument, "instrument created alongside the account")
	assert.Nil(t, got.InstrumentError)
}

// TestCreateHolderAccount_PartialFailure_201 locks the partial-failure
// contract: the account is committed, the instrument write fails, and the service
// returns a nil error, so the terminal renders 201 carrying the typed failure block.
func TestCreateHolderAccount_PartialFailure_201(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	createdAccount := &mmodel.Account{ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), AssetCode: "USD", Type: "deposit"}

	handler := &CompositionHandler{Service: composition.NewService(
		stubAccountCreator{account: createdAccount},
		stubInstrumentCreator{err: pkg.ValidateBusinessError(cn.ErrEntityNotFound, "Holder")},
	)}

	app := buildHumaCompositionApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{
		"assetCode":      "USD",
		"type":           "deposit",
		"bankingDetails": map[string]any{},
	})

	req := httptest.NewRequest(http.MethodPost, compositionURL(orgID, ledgerID, holderID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a failed instrument does not roll back the committed account")

	var got mmodel.HolderAccountResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	require.NotNil(t, got.Account, "account remains persisted on instrument failure")
	assert.Nil(t, got.Instrument)
	require.NotNil(t, got.InstrumentError, "typed failure block surfaced")
	assert.Equal(t, "FAILED", got.InstrumentError.Status)
	assert.Equal(t, cn.ErrEntityNotFound.Error(), got.InstrumentError.Reason)
}

// TestCreateHolderAccount_BadPathUUID_Direct drives the terminal's defensive
// org/ledger/holder guards, which the wired ParseUUIDPathParameters middleware makes
// unreachable through the app.
func TestCreateHolderAccount_BadPathUUID_Direct(t *testing.T) {
	t.Parallel()

	handler := &CompositionHandler{Service: composition.NewService(stubAccountCreator{}, stubInstrumentCreator{})}

	tests := []struct {
		name string
		in   *CreateHolderAccountRequest
	}{
		{
			name: "bad organization_id",
			in:   &CreateHolderAccountRequest{OrganizationID: "not-a-uuid", LedgerID: uuid.Must(libCommons.GenerateUUIDv7()).String(), ID: uuid.Must(libCommons.GenerateUUIDv7()).String()},
		},
		{
			name: "bad holder id",
			in:   &CreateHolderAccountRequest{OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(), LedgerID: uuid.Must(libCommons.GenerateUUIDv7()).String(), ID: "not-a-uuid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.CreateHolderAccount(context.Background(), tt.in)

			var detail *pkgHTTP.Detail
			require.ErrorAs(t, err, &detail, "terminal must return the canonical problem detail")
			assert.Equal(t, http.StatusBadRequest, detail.Status)
			assert.Equal(t, cn.ErrInvalidPathParameter.Error(), detail.Code)
		})
	}
}

// countingAccountCreator satisfies composition.AccountCreator, opening a distinct
// account per call. The first failures calls return err instead.
type countingAccountCreator struct {
	calls    int
	failures int
	err      error
}

func (s *countingAccountCreator) CreateAccount(_ context.Context, _, _ uuid.UUID, _ *mmodel.CreateAccountInput, _ string, _ command.RouteHolderPolicy) (*mmodel.Account, error) {
	s.calls++

	if s.calls <= s.failures {
		return nil, s.err
	}

	return &mmodel.Account{ID: uuid.Must(libCommons.GenerateUUIDv7()).String(), AssetCode: "USD", Type: "deposit"}, nil
}

// countingInstrumentCreator satisfies composition.InstrumentCreator, creating a
// distinct instrument per call, or returning err when set.
type countingInstrumentCreator struct {
	calls int
	err   error
}

func (s *countingInstrumentCreator) CreateInstrument(_ context.Context, _ string, _ uuid.UUID, _ *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	s.calls++

	if s.err != nil {
		return nil, s.err
	}

	id := uuid.Must(libCommons.GenerateUUIDv7())

	return &mmodel.Instrument{ID: &id}, nil
}

// newIdempotentCompositionHandler wires a composition handler whose slots live in slots.
func newIdempotentCompositionHandler(t *testing.T, accounts *countingAccountCreator, instruments *countingInstrumentCreator, slots *fakeCRMIdempotencyRepo) *CompositionHandler {
	t.Helper()

	return &CompositionHandler{
		Service:     composition.NewService(accounts, instruments),
		Idempotency: &services.UseCase{Idempotency: slots, Encryptor: newTestFieldEncryptor(t)},
	}
}

const (
	testCompositionAccountBody    = `{"assetCode":"USD","type":"deposit"}`
	testCompositionInstrumentBody = `{"assetCode":"USD","type":"deposit","bankingDetails":{}}`
)

// compositionAccountID extracts account.id from a decoded composition response.
func compositionAccountID(t *testing.T, got map[string]any) string {
	t.Helper()

	acc, ok := got["account"].(map[string]any)
	require.True(t, ok, "account object present: %v", got)

	id, _ := acc["id"].(string)

	return id
}

// compositionInstrumentID extracts instrument.id from a decoded composition response.
func compositionInstrumentID(t *testing.T, got map[string]any) string {
	t.Helper()

	instrument, ok := got["instrument"].(map[string]any)
	require.True(t, ok, "instrument object present: %v", got)

	id, _ := instrument["id"].(string)

	return id
}

// Scenarios "Chamadas idênticas sem header" and "Sem header, X-TTL é ignorado".
func TestCreateHolderAccount_WithoutIdempotencyKeyOpensDistinctAccounts(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	slots := newFakeCRMIdempotencyRepo()
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, slots), true)
	path := compositionURL(orgID, ledgerID, holderID)

	status1, replayed1, got1 := postCRMCreate(t, app, path, nil, testCompositionAccountBody)
	status2, replayed2, got2 := postCRMCreate(t, app, path, map[string]string{"X-TTL": "60"}, testCompositionAccountBody)

	assert.Equal(t, http.StatusCreated, status1)
	assert.Equal(t, http.StatusCreated, status2)
	assert.Equal(t, "false", replayed1)
	assert.Equal(t, "false", replayed2, "X-TTL alone claims no slot")
	assert.NotEqual(t, compositionAccountID(t, got1), compositionAccountID(t, got2), "identical calls without a key open distinct accounts")
	assert.Equal(t, 2, accounts.calls)
	assert.Empty(t, slots.store, "no X-Idempotency, no slot")
}

// A handler without the idempotency port ignores X-Idempotency.
func TestCreateHolderAccount_NilIdempotencyIgnoresKey(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	app := buildHumaCompositionApp(t, &CompositionHandler{Service: composition.NewService(accounts, &countingInstrumentCreator{})}, true)
	path := compositionURL(orgID, ledgerID, holderID)

	for attempt := 1; attempt <= 2; attempt++ {
		status, replayed, _ := postCRMCreate(t, app, path, withIdempotencyKey("c0"), testCompositionAccountBody)
		assert.Equal(t, http.StatusCreated, status, "attempt %d", attempt)
		assert.Equal(t, "false", replayed, "attempt %d", attempt)
	}

	assert.Equal(t, 2, accounts.calls)
}

// Scenario "Retentativa de composition completa".
func TestCreateHolderAccount_IdempotentReplayComplete(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	instruments := &countingInstrumentCreator{}
	slots := newFakeCRMIdempotencyRepo()
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, instruments, slots), true)
	path := compositionURL(orgID, ledgerID, holderID)

	status1, replayed1, got1 := postCRMCreate(t, app, path, withIdempotencyKey("c1"), testCompositionInstrumentBody)
	assert.Equal(t, http.StatusCreated, status1)
	assert.Equal(t, "false", replayed1)

	status2, replayed2, got2 := postCRMCreate(t, app, path, withIdempotencyKey("c1"), testCompositionInstrumentBody)
	assert.Equal(t, http.StatusCreated, status2)
	assert.Equal(t, "true", replayed2, "the retry replays the stored composition")

	assert.Equal(t, compositionAccountID(t, got1), compositionAccountID(t, got2))
	assert.Equal(t, compositionInstrumentID(t, got1), compositionInstrumentID(t, got2))
	assert.Equal(t, 1, accounts.calls, "the replay must not open another account")
	assert.Equal(t, 1, instruments.calls, "the replay must not create another instrument")
	assert.Contains(t, slots.store, services.CompositionIdempotencyKey(orgID.String(), ledgerID.String(), holderID.String(), "c1"))
}

// Scenario "Retentativa de parcial devolve a mesma conta e o mesmo instrumentError".
func TestCreateHolderAccount_IdempotentReplayPartial(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	instruments := &countingInstrumentCreator{err: pkg.ValidateBusinessError(cn.ErrEntityNotFound, "Holder")}
	slots := newFakeCRMIdempotencyRepo()
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, instruments, slots), true)
	path := compositionURL(orgID, ledgerID, holderID)

	status1, replayed1, got1 := postCRMCreate(t, app, path, withIdempotencyKey("c2"), testCompositionInstrumentBody)
	assert.Equal(t, http.StatusCreated, status1)
	assert.Equal(t, "false", replayed1)
	require.NotNil(t, got1["instrumentError"], "the first answer is a partial 201")

	status2, replayed2, got2 := postCRMCreate(t, app, path, withIdempotencyKey("c2"), testCompositionInstrumentBody)
	assert.Equal(t, http.StatusCreated, status2)
	assert.Equal(t, "true", replayed2, "the partial 201 is replayed as answered")

	assert.Equal(t, compositionAccountID(t, got1), compositionAccountID(t, got2))
	assert.Equal(t, got1["instrumentError"], got2["instrumentError"])
	assert.Nil(t, got2["instrument"])
	assert.Equal(t, 1, accounts.calls, "the replay must not open another account")
	assert.Equal(t, 1, instruments.calls, "the replay must not retry the instrument")
}

// Scenario "Asset inexistente e retentativa após correção cria a conta".
func TestCreateHolderAccount_AccountFailureReleasesSlot(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{failures: 1, err: pkg.ValidateBusinessError(cn.ErrAssetCodeNotFound, "Account")}
	slots := newFakeCRMIdempotencyRepo()
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, slots), true)
	path := compositionURL(orgID, ledgerID, holderID)
	slotKey := services.CompositionIdempotencyKey(orgID.String(), ledgerID.String(), holderID.String(), "c3")

	status, _, got := postCRMCreate(t, app, path, withIdempotencyKey("c3"), testCompositionAccountBody)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, cn.ErrAssetCodeNotFound.Error(), got["code"])
	assert.NotContains(t, slots.store, slotKey, "a failed account create must release its slot")

	status, replayed, _ := postCRMCreate(t, app, path, withIdempotencyKey("c3"), testCompositionAccountBody)
	assert.Equal(t, http.StatusCreated, status)
	assert.Equal(t, "false", replayed, "the retry ran the composition, it is not a replay")
	assert.Equal(t, 2, accounts.calls)
	assert.Contains(t, slots.store, slotKey)
}

// Scenario "Alias em uso e retentativa com a mesma chave repete o conflito".
func TestCreateHolderAccount_AliasConflictRepeatsOnRetry(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{failures: 2, err: pkg.ValidateBusinessError(cn.ErrAliasUnavailability, "Account", "conta3")}
	slots := newFakeCRMIdempotencyRepo()
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, slots), true)
	path := compositionURL(orgID, ledgerID, holderID)

	for attempt := 1; attempt <= 2; attempt++ {
		status, _, got := postCRMCreate(t, app, path, withIdempotencyKey("c5"), testCompositionAccountBody)
		assert.Equal(t, http.StatusConflict, status, "attempt %d", attempt)
		assert.Equal(t, cn.ErrAliasUnavailability.Error(), got["code"], "attempt %d must get the real conflict, never 0084", attempt)
	}

	assert.Equal(t, 2, accounts.calls)
	assert.Empty(t, slots.store)
}

// Scenario "Retentativa de composition só conta devolve a mesma conta".
func TestCreateHolderAccount_IdempotentReplayAccountOnly(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, newFakeCRMIdempotencyRepo()), true)
	path := compositionURL(orgID, ledgerID, holderID)

	_, replayed1, got1 := postCRMCreate(t, app, path, withIdempotencyKey("c2"), testCompositionAccountBody)
	status2, replayed2, got2 := postCRMCreate(t, app, path, withIdempotencyKey("c2"), testCompositionAccountBody)

	assert.Equal(t, "false", replayed1)
	assert.Equal(t, http.StatusCreated, status2)
	assert.Equal(t, "true", replayed2)
	assert.Equal(t, compositionAccountID(t, got1), compositionAccountID(t, got2))
	assert.Nil(t, got2["instrument"])
	assert.Equal(t, 1, accounts.calls)
}

// Scenario "Cache indisponível na reserva".
func TestCreateHolderAccount_ClaimFailureOpensNoAccount(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	slots := newFakeCRMIdempotencyRepo()
	slots.claimErr = errors.New("valkey unavailable")
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, slots), true)

	status, _, _ := postCRMCreate(t, app, compositionURL(orgID, ledgerID, holderID), withIdempotencyKey("c8"), testCompositionAccountBody)
	assert.GreaterOrEqual(t, status, http.StatusInternalServerError)
	assert.Equal(t, 0, accounts.calls, "an unclaimed slot must not open an account")
}

// Scenario "Requisição concorrente com a mesma chave".
func TestCreateHolderAccount_InFlightSlotConflicts(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	slots := newFakeCRMIdempotencyRepo()
	slotKey := services.CompositionIdempotencyKey(orgID.String(), ledgerID.String(), holderID.String(), "c4")
	slots.store[slotKey] = ""
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, slots), true)

	status, _, got := postCRMCreate(t, app, compositionURL(orgID, ledgerID, holderID), withIdempotencyKey("c4"), testCompositionAccountBody)
	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, cn.ErrIdempotencyKey.Error(), got["code"])
	assert.Equal(t, 0, accounts.calls, "an in-flight slot must never reach the account create")
	assert.Contains(t, slots.store, slotKey, "the conflict must not release the slot another request holds")
}

// Scenario "Mesma chave em holders diferentes".
func TestCreateHolderAccount_SameKeyOnTwoHoldersIsTwoSlots(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	holderA := uuid.Must(libCommons.GenerateUUIDv7())
	holderB := uuid.Must(libCommons.GenerateUUIDv7())

	accounts := &countingAccountCreator{}
	slots := newFakeCRMIdempotencyRepo()
	app := buildHumaCompositionApp(t, newIdempotentCompositionHandler(t, accounts, &countingInstrumentCreator{}, slots), true)

	statusA, replayedA, gotA := postCRMCreate(t, app, compositionURL(orgID, ledgerID, holderA), withIdempotencyKey("c5"), testCompositionAccountBody)
	statusB, replayedB, gotB := postCRMCreate(t, app, compositionURL(orgID, ledgerID, holderB), withIdempotencyKey("c5"), testCompositionAccountBody)

	assert.Equal(t, http.StatusCreated, statusA)
	assert.Equal(t, http.StatusCreated, statusB)
	assert.Equal(t, "false", replayedA)
	assert.Equal(t, "false", replayedB, "the same key on another holder is not a replay")
	assert.NotEqual(t, compositionAccountID(t, gotA), compositionAccountID(t, gotB))
	assert.Equal(t, 2, accounts.calls)
	assert.Contains(t, slots.store, services.CompositionIdempotencyKey(orgID.String(), ledgerID.String(), holderA.String(), "c5"))
	assert.Contains(t, slots.store, services.CompositionIdempotencyKey(orgID.String(), ledgerID.String(), holderB.String(), "c5"))
}
