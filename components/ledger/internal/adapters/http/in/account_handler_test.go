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

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
)

// buildHumaAccountApp mounts the eight account Huma operations on a /v1 group,
// faithfully mirroring the production wiring in unified-server.go: problem.Install()
// runs before any huma.Register, the Huma API is built with openapi.New over a /v1
// group, an auth-shim middleware stands in for auth.Authorize + tenant
// PostAuthMiddlewares (so the auth-preserved contract can be probed), and
// http.ParseUUIDPathParameters("account") + RegisterAccountRoutes attach the chain.
//
// MUST-NOT-PARALLELIZE (same rationale as buildHumaAssetApp): libProblem.Install()
// swaps the process-global huma.NewError hook and Huma validation uses
// process-global sync.Pools — concurrent builds/requests cross-contaminate. These
// tests are sub-second; keep them sequential.
//
// authOK=false makes the shim reject with the ledger's canonical 401 envelope
// (mirroring auth.Authorize failure) so the auth-preserved contract is testable
// without a live lib-auth server.
func buildHumaAccountApp(t *testing.T, handler *AccountHandler, authOK bool) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	apiV1 := f.Group("/v1")

	// Auth shim: stands in for auth.Authorize("midaz","accounts",verb). A rejected
	// request (authOK=false) must never reach Huma — it returns the ledger 401.
	apiV1.Use(func(c fiber.Ctx) error {
		if !authOK {
			return pkgHTTP.Unauthorized(c, "0001", "Unauthorized", "auth required")
		}

		return c.Next()
	})

	hAPI := openapi.New(f, apiV1, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v1"}})

	// Mirror the production chain: ParseUUIDPathParameters runs as a Fiber
	// middleware (no terminal handler) before the Huma terminal on each account
	// route. Registered group-relative on apiV1 so Fiber prepends /v1 — matching
	// the group-relative paths RegisterAccountRoutes registers on the Huma API.
	parse := pkgHTTP.ParseUUIDPathParameters("account")
	base := "/organizations/:organization_id/ledgers/:ledger_id/accounts"
	apiV1.Post(base, parse)
	apiV1.Patch(base+"/:id", parse)
	apiV1.Get(base, parse)
	apiV1.Get(base+"/:id", parse)
	apiV1.Get(base+"/alias/:alias", parse)
	apiV1.Get(base+"/external/:code", parse)
	apiV1.Delete(base+"/:id", parse)
	apiV1.Head(base+"/metrics/count", parse)

	RegisterAccountRoutes(hAPI, handler, routeOpSuffixV1)

	return f
}

func TestCreateAccount_Success(t *testing.T) {
	// NOT parallel: buildHumaAccountApp mutates process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	assetRepo := asset.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	ledgerRepo := ledger.NewMockRepository(ctrl)

	ledgerRepo.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), orgID, ledgerID, "", "USD").Return(true, nil).Times(1)
	accountRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, acc *mmodel.Account) (*mmodel.Account, error) {
			acc.ID = uuid.Must(libCommons.GenerateUUIDv7()).String()
			acc.OrganizationID = orgID.String()
			acc.LedgerID = ledgerID.String()
			acc.CreatedAt = fixedTestTime
			acc.UpdatedAt = fixedTestTime
			return acc, nil
		}).Times(1)
	balanceRepo.EXPECT().ExistsByAccountIDAndKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	balanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)
	// The shared body pipeline initializes Metadata to a non-nil empty map, so
	// CreateOnboardingMetadata persists it — faithful to the Fiber WithBody path.
	metadataRepo.EXPECT().Create(gomock.Any(), cn.EntityAccount, gomock.Any()).Return(nil).AnyTimes()

	handler := &AccountHandler{Command: &command.UseCase{
		AccountRepo:            accountRepo,
		AssetRepo:              assetRepo,
		OnboardingMetadataRepo: metadataRepo,
		BalanceRepo:            balanceRepo,
		LedgerRepo:             ledgerRepo,
	}}

	app := buildHumaAccountApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Account", "assetCode": "USD", "type": "deposit"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts", bytes.NewReader(body))
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
	assert.Equal(t, "Test Account", got["name"])
	assert.Equal(t, "USD", got["assetCode"])
	assert.Equal(t, orgID.String(), got["organizationId"])
	assert.Equal(t, ledgerID.String(), got["ledgerId"])
}

func TestCreateAccount_AuthPreserved(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// No repo expectations: a rejected auth must never reach the service.
	handler := &AccountHandler{Command: &command.UseCase{
		AccountRepo:            account.NewMockRepository(ctrl),
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountApp(t, handler, false)

	body, _ := json.Marshal(map[string]any{"name": "Test Account", "assetCode": "USD", "type": "deposit"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "auth middleware must reject before Huma; no public route")
}

func TestCreateAccount_ValidationError_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Missing required "assetCode"/"type" -> imperative ValidateStruct -> canonical
	// 400, service never reached.
	handler := &AccountHandler{Command: &command.UseCase{
		AccountRepo:            account.NewMockRepository(ctrl),
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Account"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts", bytes.NewReader(body))
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

func TestGetAccountByID_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(&mmodel.Account{
			ID:             accountID.String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Test Account",
			AssetCode:      "USD",
			Type:           "deposit",
			Status:         mmodel.Status{Code: "ACTIVE"},
		}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, accountID.String()).Return(nil, nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Test Account", got["name"])
	assert.Equal(t, accountID.String(), got["id"])
}

func TestGetAccountByID_BadUUID_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ParseUUIDPathParameters rejects the bad id with
	// the canonical 0065 / 400 before Huma.
	handler := &AccountHandler{Query: &query.UseCase{
		AccountRepo:            account.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
	}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/not-a-uuid", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad path UUID stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidPathParameter.Error(), got["code"])
}

func TestGetAccountByAlias_Success(t *testing.T) {
	// NOT parallel: process-global huma state. Exercises the non-UUID {alias} path
	// param: ParseUUIDPathParameters must pass it through untouched (no 422).
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountRepo.EXPECT().FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@person1").
		Return(&mmodel.Account{
			ID:             accountID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Person 1 Account",
			AssetCode:      "USD",
			Type:           "deposit",
			Alias:          testutils.Ptr("@person1"),
			Status:         mmodel.Status{Code: "ACTIVE"},
		}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, "@person1").Return(nil, nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/@person1", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "@person1", got["alias"])
}

func TestGetAccountExternalByCode_Success(t *testing.T) {
	// NOT parallel: process-global huma state. Exercises the non-UUID {code} path
	// param and the external-alias resolution shared with the by-alias op.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	externalAlias := cn.DefaultExternalAccountAliasPrefix + "BRL"

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountRepo.EXPECT().FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), externalAlias).
		Return(&mmodel.Account{
			ID:             accountID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "External BRL",
			AssetCode:      "BRL",
			Type:           "external",
			Alias:          testutils.Ptr(externalAlias),
			Status:         mmodel.Status{Code: "ACTIVE"},
		}, nil).Times(1)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, externalAlias).Return(nil, nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/external/BRL", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, externalAlias, got["alias"])
	assert.Equal(t, "BRL", got["assetCode"])
}

func TestGetAllAccounts_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Nil(), gomock.Any()).
		Return([]*mmodel.Account{}, nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts?limit=10&page=1", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "items")
	assert.EqualValues(t, 10, got["limit"])
}

func TestGetAllAccounts_BadQuery_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	// Service must never be reached: ValidateParameters rejects limit=abc with the
	// canonical 400 (ErrInvalidQueryParameter), NOT a native Huma 422.
	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: account.NewMockRepository(ctrl)}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts?limit=abc", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "bad query stays canonical 400 — no native Huma 422")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestGetAllAccounts_BadStatus_Canonical400(t *testing.T) {
	// NOT parallel: process-global huma state. Account-specific: the status enum
	// check in getAllAccounts must reject an unknown status with a canonical 400.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: account.NewMockRepository(ctrl)}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts?status=WEIRD", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid account status stays canonical 400")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), got["code"])
}

func TestDeleteAccount_204Empty(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)

	// Delete flow: Find the (non-external) account, delete its balances (none), then
	// delete the account. The account.deleted event no-ops on a nil Streaming emitter.
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil).Times(1)
	balanceRepo.EXPECT().ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).Return([]*mmodel.Balance{}, nil).Times(1)
	accountRepo.EXPECT().Delete(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).Return(nil).Times(1)

	handler := &AccountHandler{Command: &command.UseCase{AccountRepo: accountRepo, BalanceRepo: balanceRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, respBody, "DELETE 204 must have an empty body")
}

func TestCountAccounts_204WithHeader(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).Return(int64(7), nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "7", resp.Header.Get(cn.XTotalCount), "X-Total-Count header must carry the count")
	assert.Empty(t, respBody, "HEAD count must have an empty body")
	assert.Equal(t, "0", resp.Header.Get("Content-Length"), "HEAD 204 must set Content-Length: 0")
}

//
// The eight exported fiber.Ctx wrappers on AccountHandler were deleted with the
// Huma migration; the branches their tests covered in the shared cores are
// exercised here through the live Huma transport instead.

func TestUpdateAccount_Success(t *testing.T) {
	// NOT parallel: process-global huma state.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	updated := &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Updated Account Name",
		AssetCode:      "USD",
		Type:           "deposit",
		Status:         mmodel.Status{Code: "ACTIVE"},
	}

	// Command.UpdateAccount: find (external check) -> update -> metadata upsert.
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit", Name: "Original"}, nil).Times(1)
	accountRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(updated, nil).Times(1)
	metadataRepo.EXPECT().Update(gomock.Any(), cn.EntityAccount, accountID.String(), gomock.Any()).Return(nil).AnyTimes()
	// updateAccount re-reads through Query.GetAccountByID so the caller gets the
	// freshly persisted account.
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).Return(updated, nil).Times(1)
	// FindByEntity is hit twice: once by the command's metadata upsert, once by the re-read.
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, accountID.String()).Return(nil, nil).AnyTimes()

	handler := &AccountHandler{
		Command: &command.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
		Query:   &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
	}

	app := buildHumaAccountApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Account Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, "Updated Account Name", got["name"])
	assert.Equal(t, accountID.String(), got["id"])
}

func TestUpdateAccount_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. The command's not-found must reach the
	// client as the canonical 404 envelope, not a Huma-shaped error.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{
		Command: &command.UseCase{AccountRepo: accountRepo},
		Query:   &query.UseCase{AccountRepo: accountRepo},
	}

	app := buildHumaAccountApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Account Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAccountIDNotFound.Error(), got["code"])
}

func TestUpdateAccount_RetrievalError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. The update succeeds but the re-read
	// fails — updateAccount's SECOND error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil).Times(1)
	accountRepo.EXPECT().Update(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID, gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "deposit"}, nil).Times(1)
	metadataRepo.EXPECT().Update(gomock.Any(), cn.EntityAccount, accountID.String(), gomock.Any()).Return(nil).AnyTimes()
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), cn.EntityAccount, accountID.String()).Return(nil, nil).AnyTimes()
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{
		Command: &command.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
		Query:   &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo},
	}

	app := buildHumaAccountApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Updated Account Name"})
	req := httptest.NewRequest(http.MethodPatch, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAccountIDNotFound.Error(), got["code"])
}

func TestCreateAccount_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. createAccount's command-error branch:
	// an unknown asset code is a canonical 404, not a 5xx.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	assetRepo := asset.NewMockRepository(ctrl)
	ledgerRepo := ledger.NewMockRepository(ctrl)

	ledgerRepo.EXPECT().GetSettings(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	assetRepo.EXPECT().FindByNameOrCode(gomock.Any(), orgID, ledgerID, "", "ZZZ").Return(false, nil).Times(1)

	handler := &AccountHandler{Command: &command.UseCase{
		AccountRepo: account.NewMockRepository(ctrl),
		AssetRepo:   assetRepo,
		LedgerRepo:  ledgerRepo,
	}}

	app := buildHumaAccountApp(t, handler, true)

	body, _ := json.Marshal(map[string]any{"name": "Test Account", "assetCode": "ZZZ", "type": "deposit"})
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "unknown asset code is a canonical 404")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestGetAllAccounts_MetadataFilter(t *testing.T) {
	// NOT parallel: process-global huma state. Exercises getAllAccounts' metadata
	// branch (GetAllMetadataAccounts), not the plain FindAll path.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	acc1 := uuid.Must(libCommons.GenerateUUIDv7()).String()

	accountRepo := account.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	metadataRepo.EXPECT().FindList(gomock.Any(), cn.EntityAccount, gomock.Any()).
		Return([]*mongodb.Metadata{{EntityID: acc1, Data: map[string]any{"tier": "premium"}}}, nil).Times(1)
	accountRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Nil(), gomock.Any()).
		Return([]*mmodel.Account{{ID: acc1, Name: "Premium One", AssetCode: "USD", Type: "deposit"}}, nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo, OnboardingMetadataRepo: metadataRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts?metadata.tier=premium", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	items, ok := got["items"].([]any)
	require.True(t, ok, "items should be an array")
	assert.Len(t, items, 1)
}

func TestGetAllAccounts_PortfolioAndSegmentFilters(t *testing.T) {
	// NOT parallel: process-global huma state. portfolio_id/segment_id are parsed by
	// uuid.MustParse in getAllAccounts, so a valid pair must reach the query as
	// non-nil pointers (and must not panic).
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	portfolioID := uuid.Must(libCommons.GenerateUUIDv7())
	segmentID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().
		FindAll(gomock.Any(), orgID, ledgerID, gomock.Eq(&portfolioID), gomock.Eq(&segmentID), gomock.Any()).
		Return([]*mmodel.Account{}, nil).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+
			"/accounts?portfolio_id="+portfolioID.String()+"&segment_id="+segmentID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetAllAccounts_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAllAccounts' query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().FindAll(gomock.Any(), orgID, ledgerID, gomock.Nil(), gomock.Nil(), gomock.Any()).
		Return(nil, pkg.ValidateBusinessError(cn.ErrNoAccountsFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrNoAccountsFound.Error(), got["code"])
}

func TestGetAccountByID_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAccountByID's query-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAccountIDNotFound.Error(), got["code"])
}

func TestGetAccountByAlias_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. getAccountByAlias' query-error branch,
	// shared by the alias and external-by-code ops.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().FindAlias(gomock.Any(), orgID, ledgerID, gomock.Nil(), "@missing").
		Return(nil, pkg.ValidateBusinessError(cn.ErrAccountAliasNotFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/alias/@missing", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Contains(t, got, "code")
}

func TestDeleteAccount_ServiceError_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state. deleteAccount's command-error branch.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, gomock.Nil(), accountID).
		Return(nil, pkg.ValidateBusinessError(cn.ErrAccountIDNotFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{Command: &command.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodDelete, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String(), nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
	assert.Equal(t, cn.ErrAccountIDNotFound.Error(), got["code"])
}

func TestCountAccounts_ServiceError(t *testing.T) {
	// NOT parallel: process-global huma state. countAccounts' query-error branch: the
	// HEAD op must surface the canonical status with no body.
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	accountRepo := account.NewMockRepository(ctrl)
	accountRepo.EXPECT().Count(gomock.Any(), orgID, ledgerID).
		Return(int64(0), pkg.ValidateBusinessError(cn.ErrNoAccountsFound, cn.EntityAccount)).Times(1)

	handler := &AccountHandler{Query: &query.UseCase{AccountRepo: accountRepo}}

	app := buildHumaAccountApp(t, handler, true)

	req := httptest.NewRequest(http.MethodHead, "/v1/organizations/"+orgID.String()+"/ledgers/"+ledgerID.String()+"/accounts/metrics/count", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Empty(t, resp.Header.Get(cn.XTotalCount), "a failed count must not advertise a total")
}
