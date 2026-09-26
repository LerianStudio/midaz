// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the transport gate of the closing command (AC-01 to AC-04). Every
// test drives the PRODUCTION registrar — RegisterAccountV2RoutesToApp — so what is
// pinned is the surface the deployed binary serves: the guard chain, the UUID
// rejection, the bodiless 204 and the refusal codes the command produces, carried
// out untouched by the handler.
//
// MUST-NOT-PARALLELIZE: libProblem.Install swaps a process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools, so concurrent builds
// cross-contaminate.

var (
	closeRouteOrgID     = uuid.MustParse("eeeeeeee-0000-0000-0000-000000000001")
	closeRouteLedgerID  = uuid.MustParse("eeeeeeee-0000-0000-0000-000000000002")
	closeRouteAccountID = uuid.MustParse("eeeeeeee-0000-0000-0000-000000000003")
	closeRouteBalanceID = uuid.MustParse("eeeeeeee-0000-0000-0000-000000000004")

	closeRouteInstant = time.Date(2026, 9, 18, 10, 30, 0, 0, time.UTC)
)

// closeRouteMocks holds the repositories the closing command reads, so a transport
// test can program the outcome the handler has to carry.
type closeRouteMocks struct {
	account     *account.MockRepository
	balance     *balance.MockRepository
	operation   *operation.MockRepository
	transaction *transaction.MockRepository
	redis       *txRedis.MockRedisRepository
}

// newCloseRouteHandler builds the account handler over a real command use case
// whose repositories are mocked, so the path under test is the production one from
// the Huma terminal down to the command.
func newCloseRouteHandler(t *testing.T) (*AccountHandler, *closeRouteMocks) {
	t.Helper()

	ctrl := gomock.NewController(t)

	mocks := &closeRouteMocks{
		account:     account.NewMockRepository(ctrl),
		balance:     balance.NewMockRepository(ctrl),
		operation:   operation.NewMockRepository(ctrl),
		transaction: transaction.NewMockRepository(ctrl),
		redis:       txRedis.NewMockRedisRepository(ctrl),
	}

	handler := &AccountHandler{
		Command: &command.UseCase{
			AccountRepo:          mocks.account,
			BalanceRepo:          mocks.balance,
			OperationRepo:        mocks.operation,
			TransactionRepo:      mocks.transaction,
			TransactionRedisRepo: mocks.redis,
		},
	}

	return handler, mocks
}

// expectEligibleClosing programs the whole happy path of one closing: the
// authoritative read, the protection, the settled balances, the concluded work, the
// absent pending, the write and the finalization.
func (m *closeRouteMocks) expectEligibleClosing() {
	zeroed := &mmodel.Balance{
		ID:             closeRouteBalanceID.String(),
		AccountID:      closeRouteAccountID.String(),
		OrganizationID: closeRouteOrgID.String(),
		LedgerID:       closeRouteLedgerID.String(),
		Alias:          "@closing",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		OverdraftUsed:  decimal.Zero,
		Version:        1,
	}

	m.account.EXPECT().Find(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, nil, closeRouteAccountID, mmodel.HolderOffV1).
		Return(&mmodel.Account{
			ID:             closeRouteAccountID.String(),
			OrganizationID: closeRouteOrgID.String(),
			LedgerID:       closeRouteLedgerID.String(),
			Type:           "deposit",
		}, nil)

	// The closing installs its marker first, recognizes it as its own, and then
	// takes the ownership under the same token.
	var token string

	m.redis.EXPECT().AcquireAccountClosingMarker(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, installed string) (bool, error) {
			token = installed

			return true, nil
		})
	m.redis.EXPECT().GetAccountClosingMarker(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, bool, error) {
			return token, true, nil
		})
	m.redis.EXPECT().AcquireAccountAdminOwnership(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID, gomock.Any()).
		Return(true, nil)

	m.balance.EXPECT().ListByAccountID(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID).
		Return([]*mmodel.Balance{zeroed}, nil)
	m.redis.EXPECT().ListBalanceByKey(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, gomock.Any()).
		Return(nil, redis.Nil)
	m.redis.EXPECT().ScanRecoveryMessages(gomock.Any(), gomock.Any(), uint64(0), gomock.Any()).
		Return(txRedis.RecoveryScanPage{Cursor: 0}, nil).Times(2)
	m.operation.EXPECT().ListLatestByBalances(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, gomock.Any()).
		Return(map[string]*operation.Operation{}, nil)
	m.transaction.EXPECT().HasPendingByAccount(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID).
		Return(false, nil)

	m.redis.EXPECT().MarkAccountClosingWriteIssued(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID, gomock.Any()).
		Return(true, nil)
	m.account.EXPECT().CloseAccount(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID).
		Return(closeRouteInstant, nil)
	m.redis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	m.redis.EXPECT().SetAccountClosedMarker(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID, closeRouteInstant).
		Return(nil)
	m.redis.EXPECT().ReleaseAccountClosingAttempt(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID, gomock.Any()).
		Return(true, nil)
	m.redis.EXPECT().ReleaseAccountAdminOwnership(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, closeRouteAccountID, gomock.Any()).
		Return(true, nil)
}

// mountCloseAccountRoute builds a /v2 group carrying the PRODUCTION account
// registrar over handler, mirroring the unified server: problem.Install before any
// huma.Register, the ErrorEnvelope on the app root, and the Fiber guard chain the
// registrar attaches itself.
func mountCloseAccountRoute(t *testing.T, auth *middleware.AuthClient, handler *AccountHandler) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()

	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group("/v2")
	api := openapi.New(f, group, openapi.Config{
		Title: "account-close", Version: "test", Servers: []string{"/v2"},
	})
	pkgHTTP.InstallLedgerSchemaNamer(api)
	openapi.DeclareBearerAuth(api)

	RegisterAccountV2RoutesToApp(group, api, auth, handler, nil)

	return f
}

// closeAccountRoutePath builds the request path for an arbitrary account
// identifier, so a malformed one can be presented to the same route.
func closeAccountRoutePath(accountID string) string {
	return "/v2/organizations/" + closeRouteOrgID.String() +
		"/ledgers/" + closeRouteLedgerID.String() +
		"/accounts/" + accountID + "/close"
}

// postCloseAccount sends the closing command — no body, as the contract has none —
// and returns the response.
func postCloseAccount(t *testing.T, app *fiber.App, accountID string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodPost, closeAccountRoutePath(accountID), nil)
	req.Header.Set("Authorization", "Bearer "+guardBearerToken(t))

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

// TestCloseAccountV2_EligibleAccountGets204 covers AC-01: an eligible account
// closes and the caller receives a bodiless 204. The body is asserted empty because
// the instant is NOT on the wire — it is read back as the account's closedAt.
func TestCloseAccountV2_EligibleAccountGets204(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, mocks := newCloseRouteHandler(t)
	mocks.expectEligibleClosing()

	app := mountCloseAccountRoute(t, &middleware.AuthClient{Enabled: false}, handler)

	resp := postCloseAccount(t, app, closeRouteAccountID.String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Empty(t, body, "the closing answers 204 with no body")
}

// TestCloseAccountV2_AlreadyClosedKeepsTheCommandVerdict covers the refusal half of
// AC-01: the handler carries the command's verdict out untouched — the conflict
// class and the sentinel are the command's, not a code the transport rewrote.
func TestCloseAccountV2_AlreadyClosedKeepsTheCommandVerdict(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, mocks := newCloseRouteHandler(t)

	closedAt := closeRouteInstant
	mocks.account.EXPECT().Find(gomock.Any(), closeRouteOrgID, closeRouteLedgerID, nil, closeRouteAccountID, mmodel.HolderOffV1).
		Return(&mmodel.Account{
			ID:             closeRouteAccountID.String(),
			OrganizationID: closeRouteOrgID.String(),
			LedgerID:       closeRouteLedgerID.String(),
			Type:           "deposit",
			ClosedAt:       &closedAt,
		}, nil)

	app := mountCloseAccountRoute(t, &middleware.AuthClient{Enabled: false}, handler)

	resp := postCloseAccount(t, app, closeRouteAccountID.String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), cn.ErrAccountAlreadyClosed.Error(),
		"the problem document must carry the command's own code")
}

// TestCloseAccountV2_MalformedAccountIDGets400 covers the first half of AC-03: the
// path param is named account_id, so the shared ParseUUIDPathParameters middleware
// rejects a malformed identifier with the canonical 400 BEFORE the command runs —
// the handler carries no repository expectation here, so reaching it would fail.
func TestCloseAccountV2_MalformedAccountIDGets400(t *testing.T) {
	// NOT parallel: process-global huma state.
	handler, _ := newCloseRouteHandler(t)

	app := mountCloseAccountRoute(t, &middleware.AuthClient{Enabled: false}, handler)

	resp := postCloseAccount(t, app, "not-a-uuid")
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), cn.ErrInvalidPathParameter.Error())
	assert.Contains(t, string(body), "account_id", "the rejection must name the offending param")
}

// TestCloseAccountV2_DeniedWithoutTheAccountsGrant covers AC-02: the route is
// behind the guard chain, and the tuple it forwards is (midaz, accounts, post). A
// denied caller is answered 403 without the command running — the handler carries a
// nil command here, so reaching it would panic.
func TestCloseAccountV2_DeniedWithoutTheAccountsGrant(t *testing.T) {
	// NOT parallel: process-global huma state.
	var call authzCall

	srv := newAuthzTupleCapture(t, &call, false)
	defer srv.Close()

	auth := &middleware.AuthClient{Address: srv.URL, Enabled: true}

	app := mountCloseAccountRoute(t, auth, &AccountHandler{})

	resp := postCloseAccount(t, app, closeRouteAccountID.String())
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"a caller without the accounts grant must be denied")
	assert.Equal(t, midazName, call.product)
	assert.Equal(t, "accounts", call.resource, "closing is an operation on the account resource")
	assert.Equal(t, "post", call.action)
}

// TestCloseAccountRoute_PublishedOnV2Only covers AC-04: the contract publishes the
// closing on /v2 alone. A /v1 twin would be a deprecated copy of a command that
// never shipped there, and the protection of a closed account does not depend on
// it — a movement is refused on both contracts.
func TestCloseAccountRoute_PublishedOnV2Only(t *testing.T) {
	t.Parallel()

	app, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/close"

	item, ok := paths["/v2"+opPath]
	require.True(t, ok, "the /v2 surface must publish the closing command")

	operation := operationForMethod(item, http.MethodPost)
	require.NotNil(t, operation, "the op must be a POST")
	assert.Equal(t, "closeAccount"+v2OpSuffix, operation.OperationID)
	assert.Nil(t, operation.RequestBody, "the closing command takes no body")

	_, onV1 := paths["/v1"+opPath]
	assert.False(t, onV1, "the closing command must NOT be published on /v1")

	for _, unexpected := range []*huma.Operation{item.Get, item.Put, item.Patch, item.Delete, item.Head} {
		assert.Nil(t, unexpected, "POST is the only operation on the closing path")
	}

	// The published contract and the SERVED surface are two different claims: the
	// /v1 guard chain is shared by every account route, so a closing path attached
	// to it would answer on /v1 with no operation to document it.
	req := httptest.NewRequest(fiber.MethodPost, "/v1/organizations/"+closeRouteOrgID.String()+
		"/ledgers/"+closeRouteLedgerID.String()+
		"/accounts/"+closeRouteAccountID.String()+"/close", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the /v1 mount must serve no closing route")
}
