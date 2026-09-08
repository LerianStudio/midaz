// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// buildHumaBlockExceptionApp mounts the block-exception Huma terminal on a /v2
// group, mirroring the production wiring but standing an always-pass shim in for
// auth.Authorize so the transport contract is probeable without a live lib-auth
// server. The authz tuple itself is covered separately, against the real
// registrar, in account_block_exception_routes_test.go.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global
// huma.NewError hook and Huma validation uses process-global sync.Pools —
// concurrent builds cross-contaminate. These cases are sub-second.
func buildHumaBlockExceptionApp(t *testing.T, handler *AccountBlockExceptionHandler) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v2 serves the RFC 9457 document these assertions read.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group("/v2")

	const fiberPath = "/organizations/:organization_id/ledgers/:ledger_id/accounts/block-exceptions"

	routePost(group, fiberPath, []fiber.Handler{
		func(c fiber.Ctx) error { return c.Next() },
		pkgHTTP.ParseUUIDPathParameters("account_block_exception"),
	})

	api := openapi.New(f, group, openapi.Config{
		Title: "block-exception-handler", Version: "test", Servers: []string{"/v2"},
	})
	pkgHTTP.InstallLedgerSchemaNamer(api)
	openapi.DeclareBearerAuth(api)

	RegisterAccountBlockExceptionRoutes(api, handler, v2OpSuffix)

	return f
}

// blockExceptionPath builds the request path for the given raw path params, so a
// case can pass a deliberately malformed UUID.
func blockExceptionPath(orgID, ledgerID string) string {
	return "/v2/organizations/" + orgID + "/ledgers/" + ledgerID + "/accounts/block-exceptions"
}

// postBlockExceptions drives one POST through app and returns the status and the
// decoded error envelope's "code" (empty when the body carries none).
func postBlockExceptions(t *testing.T, app *fiber.App, path, body string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var envelope struct {
		Code string `json:"code"`
	}

	// A 201 carries the success body, which has no "code"; decode failures are
	// therefore not an error here.
	_ = json.Unmarshal(raw, &envelope)

	return resp.StatusCode, envelope.Code
}

// TestCreateAccountBlockExceptions_TransportRejections sweeps every rejection
// the transport must produce BEFORE the command runs, plus the ones the command
// produces from a well-formed body. No collaborator is mocked, so a case that
// reached the command's collaborators would panic on a nil repo rather than pass.
func TestCreateAccountBlockExceptions_TransportRejections(t *testing.T) {
	orgID, ledgerID := uuid.NewString(), uuid.NewString()
	oversizedBatch := blockExceptionBatchBody(t, mmodel.AccountBlockExceptionMaxBatchSize+1)

	tests := []struct {
		name     string
		path     string
		body     string
		wantCode int
		wantErr  error
	}{
		{
			name:     "malformed organization id",
			path:     blockExceptionPath("not-a-uuid", ledgerID),
			body:     blockExceptionBody,
			wantCode: fiber.StatusBadRequest,
			wantErr:  constant.ErrInvalidPathParameter,
		},
		{
			name:     "malformed ledger id",
			path:     blockExceptionPath(orgID, "not-a-uuid"),
			body:     blockExceptionBody,
			wantCode: fiber.StatusBadRequest,
			wantErr:  constant.ErrInvalidPathParameter,
		},
		{
			name:     "unparseable json",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     `{"exceptions":`,
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "unknown field",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     `{"exceptions":[{"accountAlias":"@a","amount":"1"}],"bypassEverything":true}`,
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "missing exceptions",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     `{}`,
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "empty exceptions",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     `{"exceptions":[]}`,
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "batch above the limit",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     oversizedBatch,
			wantCode: fiber.StatusBadRequest,
			wantErr:  constant.ErrAccountBlockExceptionsBatchTooLarge,
		},
		{
			name:     "negative amount",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     `{"exceptions":[{"accountAlias":"@a","amount":"-1"}]}`,
			wantCode: fiber.StatusBadRequest,
			wantErr:  constant.ErrAccountBlockExceptionInvalidAmount,
		},
		{
			name:     "ttl above the cap",
			path:     blockExceptionPath(orgID, ledgerID),
			body:     `{"exceptions":[{"accountAlias":"@a","amount":"1","ttl":` + strconv.Itoa(mmodel.AccountBlockExceptionMaxTTLSeconds+1) + `}]}`,
			wantCode: fiber.StatusBadRequest,
			wantErr:  constant.ErrAccountBlockExceptionInvalidTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := buildHumaBlockExceptionApp(t, &AccountBlockExceptionHandler{
				Command: &command.UseCase{},
			})

			status, code := postBlockExceptions(t, app, tt.path, tt.body)

			assert.Equal(t, tt.wantCode, status)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr.Error(), code, "the envelope must carry the registry code")
			}
		})
	}
}

// TestCreateAccountBlockExceptions_UnknownAliasIsNotFound pins the status of the
// alias rejection: an unknown alias is a MISSING entity (404), not a malformed
// request, and it rejects the whole batch.
func TestCreateAccountBlockExceptions_UnknownAliasIsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	accountRepo := account.NewMockRepository(ctrl)

	accountRepo.EXPECT().
		ListAccountsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	app := buildHumaBlockExceptionApp(t, &AccountBlockExceptionHandler{
		Command: &command.UseCase{AccountRepo: accountRepo},
	})

	status, code := postBlockExceptions(t, app,
		blockExceptionPath(uuid.NewString(), uuid.NewString()), blockExceptionBody)

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.Equal(t, constant.ErrAccountBlockExceptionAliasNotFound.Error(), code)
}

// TestCreateAccountBlockExceptions_Created201 locks the success contract of the
// transport: 201, and the response envelope carries one entry per requested
// exception with its identifier and derived expiry.
func TestCreateAccountBlockExceptions_Created201(t *testing.T) {
	ctrl := gomock.NewController(t)
	accountRepo := account.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	alias := "@fraud_account"

	accountRepo.EXPECT().
		ListAccountsByAlias(gomock.Any(), gomock.Any(), gomock.Any(), []string{alias}).
		Return([]*mmodel.Account{{ID: uuid.NewString(), Alias: &alias}}, nil)
	redisRepo.EXPECT().
		CreateAccountBlockExceptions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Len(1)).
		Return(nil)

	app := buildHumaBlockExceptionApp(t, &AccountBlockExceptionHandler{
		Command: &command.UseCase{AccountRepo: accountRepo, TransactionRedisRepo: redisRepo},
	})

	req := httptest.NewRequest(fiber.MethodPost,
		blockExceptionPath(uuid.NewString(), uuid.NewString()), strings.NewReader(blockExceptionBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body mmodel.AccountBlockExceptions
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Exceptions, 1)

	assert.Equal(t, alias, body.Exceptions[0].AccountAlias)
	assert.Equal(t, "150.00", body.Exceptions[0].Amount)

	_, parseErr := uuid.Parse(body.Exceptions[0].AccountBlockExceptionID)
	assert.NoError(t, parseErr, "the identifier must be a UUID")
	assert.False(t, body.Exceptions[0].ExpiresAt.IsZero())
}

// blockExceptionBatchBody builds a JSON batch of n valid items.
func blockExceptionBatchBody(t *testing.T, n int) string {
	t.Helper()

	items := make([]mmodel.CreateAccountBlockExceptionInput, 0, n)
	for i := range n {
		items = append(items, mmodel.CreateAccountBlockExceptionInput{
			AccountAlias: "@alias_" + strconv.Itoa(i),
			Amount:       "1",
		})
	}

	raw, err := json.Marshal(mmodel.CreateAccountBlockExceptionsInput{Exceptions: items})
	require.NoError(t, err)

	return string(raw)
}
