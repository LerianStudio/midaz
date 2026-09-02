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
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accountexception"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// accountExceptionRepos bundles the mock repositories the account-exception command and
// query use cases depend on, so a test can set expectations on each seam directly.
type accountExceptionRepos struct {
	accountRepo   *account.MockRepository
	exceptionRepo *accountexception.MockRepository
}

// newAccountExceptionHandler builds an AccountExceptionHandler whose command and query use
// cases are wired to mock repositories, so the HTTP shells are exercised over the REAL use
// cases rather than a stubbed seam. Streaming is left nil: EmitBrokerBestEffort no-ops on a
// nil emitter, so the best-effort event path stays inert in these transport tests.
func newAccountExceptionHandler(t *testing.T) (*AccountExceptionHandler, *accountExceptionRepos) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repos := &accountExceptionRepos{
		accountRepo:   account.NewMockRepository(ctrl),
		exceptionRepo: accountexception.NewMockRepository(ctrl),
	}

	handler := &AccountExceptionHandler{
		Command: &command.UseCase{
			AccountRepo:          repos.accountRepo,
			AccountExceptionRepo: repos.exceptionRepo,
		},
		Query: &query.UseCase{
			AccountExceptionRepo: repos.exceptionRepo,
		},
	}

	return handler, repos
}

// buildHumaAccountExceptionApp mounts ONE version group of the account-exception surface,
// faithfully mirroring the production wiring: problem.Install before any huma.Register, the
// Huma API over the version group, ParseUUIDPathParameters as a Fiber middleware on the five
// paths, then the version's Huma terminals.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError hook and
// Huma validation uses process-global sync.Pools.
func buildHumaAccountExceptionApp(t *testing.T, handler *AccountExceptionHandler, version string, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group(version)

	group.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	api := openapi.New(f, group, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{version}})

	parse := pkgHTTP.ParseUUIDPathParameters("account_exception")
	base := "/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/exceptions"
	group.Post(base, parse)
	group.Get(base, parse)
	group.Get(base+"/:exception_id", parse)
	group.Patch(base+"/:exception_id", parse)
	group.Delete(base+"/:exception_id", parse)

	suffix := v1OpSuffix
	if version == "/v2" {
		suffix = v2OpSuffix
	}

	RegisterAccountExceptionRoutes(api, handler, suffix)

	return f
}

// exceptionableAccount is the pre-state AccountRepo.Find returns for the happy path: a plain
// deposit account an exception can be registered against.
func exceptionableAccount(orgID, ledgerID, accountID uuid.UUID, accountType string) *mmodel.Account {
	return &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Exceptionable Account",
		Type:           accountType,
		AssetCode:      "USD",
	}
}

func exceptionsBasePath(orgID, ledgerID, accountID uuid.UUID) string {
	return "/v1/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/accounts/" + accountID.String() + "/exceptions"
}

func TestCreateAccountException_Success(t *testing.T) {
	// NOT parallel: buildHumaAccountExceptionApp mutates process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Any(), accountID, gomock.Any()).
		Return(exceptionableAccount(orgID, ledgerID, accountID, "deposit"), nil).Times(1)

	repos.exceptionRepo.EXPECT().Create(gomock.Any(), orgID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, e *mmodel.AccountException) (*mmodel.AccountException, error) {
			e.CreatedAt = fixedTestTime
			e.UpdatedAt = fixedTestTime
			return e, nil
		}).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	body, _ := json.Marshal(map[string]any{"operationalTypeCodes": []string{"PIX_IN", "TED_OUT"}, "context": "Judicial order 12345/2026"})
	req := httptest.NewRequest(http.MethodPost, exceptionsBasePath(orgID, ledgerID, accountID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(respBody))
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Judicial order 12345/2026", got["context"])
	assert.Equal(t, accountID.String(), got["accountId"])
}

func TestCreateAccountException_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state. No repo expectations: rejected auth must never
	// reach the use case.
	handler, _ := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	app := buildHumaAccountExceptionApp(t, handler, "/v1", false)

	body, _ := json.Marshal(map[string]any{"operationalTypeCodes": []string{"PIX_IN"}, "context": "reason"})
	req := httptest.NewRequest(http.MethodPost, exceptionsBasePath(orgID, ledgerID, accountID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateAccountException_ExternalAccount_422(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Any(), accountID, gomock.Any()).
		Return(exceptionableAccount(orgID, ledgerID, accountID, "external"), nil).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	body, _ := json.Marshal(map[string]any{"operationalTypeCodes": []string{"PIX_IN"}, "context": "reason"})
	req := httptest.NewRequest(http.MethodPost, exceptionsBasePath(orgID, ledgerID, accountID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "body: %s", string(respBody))
	assert.Contains(t, string(respBody), constant.ErrForbiddenExternalAccountManipulation.Error())
}

func TestCreateAccountException_AccountNotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Any(), accountID, gomock.Any()).
		Return(nil, nil).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	body, _ := json.Marshal(map[string]any{"operationalTypeCodes": []string{"PIX_IN"}, "context": "reason"})
	req := httptest.NewRequest(http.MethodPost, exceptionsBasePath(orgID, ledgerID, accountID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCreateAccountException_InvalidBody_400(t *testing.T) {
	// NOT parallel: process-global huma state. No repo expectations: body validation fails
	// before the use case is reached.
	handler, _ := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	// operationalTypeCodes empty violates min=1; the imperative DecodeAndValidate rejects it.
	body, _ := json.Marshal(map[string]any{"operationalTypeCodes": []string{}, "context": "reason"})
	req := httptest.NewRequest(http.MethodPost, exceptionsBasePath(orgID, ledgerID, accountID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetAccountExceptionByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	exceptionID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, accountID, exceptionID).
		Return(&mmodel.AccountException{
			ID:                   exceptionID.String(),
			OrganizationID:       orgID.String(),
			LedgerID:             ledgerID.String(),
			AccountID:            accountID.String(),
			OperationalTypeCodes: []string{"PIX_IN"},
			Context:              "reason",
			CreatedAt:            fixedTestTime,
			UpdatedAt:            fixedTestTime,
		}, nil).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	req := httptest.NewRequest(http.MethodGet, exceptionsBasePath(orgID, ledgerID, accountID)+"/"+exceptionID.String(), nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got))
	assert.Equal(t, exceptionID.String(), got["id"])
}

func TestGetAccountExceptionByID_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	exceptionID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, accountID, exceptionID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	req := httptest.NewRequest(http.MethodGet, exceptionsBasePath(orgID, ledgerID, accountID)+"/"+exceptionID.String(), nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestListAccountExceptions_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().FindAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
		Return([]*mmodel.AccountException{
			{ID: uuid.NewString(), AccountID: accountID.String(), OperationalTypeCodes: []string{"PIX_IN"}, Context: "reason"},
		}, nil).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	req := httptest.NewRequest(http.MethodGet, exceptionsBasePath(orgID, ledgerID, accountID), nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.Contains(t, string(respBody), "items")
}

func TestListAccountExceptions_Empty_404(t *testing.T) {
	// NOT parallel: process-global huma state. An empty page is the 0504 business condition.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().FindAllByAccountID(gomock.Any(), orgID, ledgerID, accountID, gomock.Any()).
		Return([]*mmodel.AccountException{}, nil).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	req := httptest.NewRequest(http.MethodGet, exceptionsBasePath(orgID, ledgerID, accountID), nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateAccountException_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	exceptionID := uuid.Must(libCommons.GenerateUUIDv7())

	current := &mmodel.AccountException{
		ID:                   exceptionID.String(),
		OrganizationID:       orgID.String(),
		LedgerID:             ledgerID.String(),
		AccountID:            accountID.String(),
		OperationalTypeCodes: []string{"PIX_IN"},
		Context:              "old",
		CreatedAt:            fixedTestTime,
		UpdatedAt:            fixedTestTime,
	}

	repos.exceptionRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, accountID, exceptionID).Return(current, nil).Times(1)
	repos.exceptionRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, accountID, exceptionID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, _ uuid.UUID, patch *mmodel.AccountException) (*mmodel.AccountException, error) {
			current.Context = patch.Context
			return current, nil
		}).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	body, _ := json.Marshal(map[string]any{"context": "new reason"})
	req := httptest.NewRequest(http.MethodPatch, exceptionsBasePath(orgID, ledgerID, accountID)+"/"+exceptionID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))
	assert.Contains(t, string(respBody), "new reason")
}

func TestUpdateAccountException_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	exceptionID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().FindByID(gomock.Any(), orgID, ledgerID, accountID, exceptionID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	body, _ := json.Marshal(map[string]any{"context": "new reason"})
	req := httptest.NewRequest(http.MethodPatch, exceptionsBasePath(orgID, ledgerID, accountID)+"/"+exceptionID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeleteAccountException_NotFound_404(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	exceptionID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, accountID, exceptionID).
		Return(services.ErrDatabaseItemNotFound).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	req := httptest.NewRequest(http.MethodDelete, exceptionsBasePath(orgID, ledgerID, accountID)+"/"+exceptionID.String(), nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestAccountExceptionHandlers_BadPathParams_Canonical400 drives the defensive path-parameter
// guards in the Huma handlers directly with malformed UUID strings, covering the parse-error
// branches the wired ParseUUIDPathParameters middleware normally short-circuits before the
// terminal. Every case must return a non-nil HumaProblem error and never touch a use case, so
// the handler is wired to nil use cases: reaching one would nil-panic.
func TestAccountExceptionHandlers_BadPathParams_Canonical400(t *testing.T) {
	t.Parallel()

	handler := &AccountExceptionHandler{}
	ctx := context.Background()
	good := uuid.NewString()

	t.Run("create bad organization_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.CreateAccountException(ctx, &CreateAccountExceptionRequest{OrganizationID: "not-a-uuid", LedgerID: good, AccountID: good, RawBody: []byte("{}")})
		require.Error(t, err)
	})

	t.Run("create bad account_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.CreateAccountException(ctx, &CreateAccountExceptionRequest{OrganizationID: good, LedgerID: good, AccountID: "not-a-uuid", RawBody: []byte("{}")})
		require.Error(t, err)
	})

	t.Run("list bad organization_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.GetAllAccountExceptions(ctx, &ListAccountExceptionsRequest{OrganizationID: "not-a-uuid", LedgerID: good, AccountID: good})
		require.Error(t, err)
	})

	t.Run("list bad account_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.GetAllAccountExceptions(ctx, &ListAccountExceptionsRequest{OrganizationID: good, LedgerID: good, AccountID: "not-a-uuid"})
		require.Error(t, err)
	})

	t.Run("getByID bad exception_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.GetAccountExceptionByID(ctx, &GetAccountExceptionRequest{OrganizationID: good, LedgerID: good, AccountID: good, ExceptionID: "not-a-uuid"})
		require.Error(t, err)
	})

	t.Run("update bad exception_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.UpdateAccountException(ctx, &UpdateAccountExceptionRequest{OrganizationID: good, LedgerID: good, AccountID: good, ExceptionID: "not-a-uuid", RawBody: []byte("{}")})
		require.Error(t, err)
	})

	t.Run("delete bad exception_id", func(t *testing.T) {
		t.Parallel()
		_, err := handler.DeleteAccountExceptionByID(ctx, &GetAccountExceptionRequest{OrganizationID: good, LedgerID: good, AccountID: good, ExceptionID: "not-a-uuid"})
		require.Error(t, err)
	})

	t.Run("update invalid body", func(t *testing.T) {
		t.Parallel()
		_, err := handler.UpdateAccountException(ctx, &UpdateAccountExceptionRequest{OrganizationID: good, LedgerID: good, AccountID: good, ExceptionID: good, RawBody: []byte("{invalid")})
		require.Error(t, err)
	})
}

func TestDeleteAccountException_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, repos := newAccountExceptionHandler(t)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	exceptionID := uuid.Must(libCommons.GenerateUUIDv7())

	repos.exceptionRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, accountID, exceptionID).Return(nil).Times(1)

	app := buildHumaAccountExceptionApp(t, handler, "/v1", true)

	req := httptest.NewRequest(http.MethodDelete, exceptionsBasePath(orgID, ledgerID, accountID)+"/"+exceptionID.String(), nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
