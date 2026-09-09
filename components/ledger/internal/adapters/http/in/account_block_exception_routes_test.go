// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// This file is the Task 2.4 gate: the route must be unreachable without its OWN
// grant. It drives the PRODUCTION registrar through a capturing authz server, so
// the recorded (product, resource, action) tuple is the one the deployed binary
// sends, and a registrar repointed at "accounts" — the mistake that would let an
// account-CRUD grant mint a block bypass — makes the recorded resource diverge.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook and
// Huma validation uses process-global sync.Pools; concurrent builds
// cross-contaminate.

// authzCall is one recorded authorization request.
type authzCall struct {
	product  string
	resource string
	action   string
}

// newAuthzTupleCapture returns an httptest server standing in for the authz
// service. It records the forwarded (product, resource, action) into *call and
// answers with the caller's decision, so the same helper drives both the denied
// and the granted case.
func newAuthzTupleCapture(t *testing.T, call *authzCall, authorized bool) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authz capture: decode request body: %v", err)
		}

		*call = authzCall{product: body["product"], resource: body["resource"], action: body["action"]}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		payload := `{"authorized":false}`
		if authorized {
			payload = `{"authorized":true}`
		}

		if _, err := w.Write([]byte(payload)); err != nil {
			t.Errorf("authz capture: write response: %v", err)
		}
	}))
}

// mountBlockExceptionRoute builds a /v2 group carrying the PRODUCTION registrar
// over handler, and returns the app plus the request path.
func mountBlockExceptionRoute(t *testing.T, auth *middleware.AuthClient, handler *AccountBlockExceptionHandler) (*fiber.App, string) {
	t.Helper()

	orgID, ledgerID := uuid.New(), uuid.New()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	libProblem.Install()

	// Mirror production: the ledger registers ErrorEnvelope on the app root.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group("/v2")
	api := openapi.New(f, group, openapi.Config{
		Title: "authz-guard", Version: "test", Servers: []string{"/v2"},
	})
	pkgHTTP.InstallLedgerSchemaNamer(api)
	openapi.DeclareBearerAuth(api)

	RegisterAccountBlockExceptionV2RoutesToApp(group, api, auth, handler, nil)

	return f, "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/accounts/block-exceptions"
}

// blockExceptionBody is a minimal valid create batch.
const blockExceptionBody = `{"exceptions":[{"accountAlias":"@fraud_account","amount":"150.00"}]}`

// TestAuthz_AccountBlockExceptions_DeniedWithoutDedicatedGrant is the negative
// half of the RBAC gate: the route reaches auth, is denied, and answers 403
// WITHOUT running the business terminal (a zero-value handler would panic on the
// nil command if it did).
func TestAuthz_AccountBlockExceptions_DeniedWithoutDedicatedGrant(t *testing.T) {
	var call authzCall

	srv := newAuthzTupleCapture(t, &call, false)
	defer srv.Close()

	auth := &middleware.AuthClient{Address: srv.URL, Enabled: true}

	f, path := mountBlockExceptionRoute(t, auth, &AccountBlockExceptionHandler{})

	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(blockExceptionBody))
	req.Header.Set("Authorization", "Bearer "+guardBearerToken(t))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode,
		"a caller without the dedicated grant must be denied")
	assert.Equal(t, midazName, call.product, "the route authorizes under the midaz appName")
	assert.Equal(t, accountBlockExceptionResource, call.resource,
		"the route must authorize under its OWN resource, never under accounts")
	assert.Equal(t, "post", call.action)
}

// TestAuthz_AccountBlockExceptions_CreatedWithDedicatedGrant is the positive
// half: the SAME production registrar, granted, runs the terminal and answers
// 201 with one identifier per requested exception. The command is real; only its
// two collaborators are mocked, so the 201 proves the whole chain (auth ->
// UUID path parse -> Huma decode -> command -> cache write) is wired.
func TestAuthz_AccountBlockExceptions_CreatedWithDedicatedGrant(t *testing.T) {
	var call authzCall

	srv := newAuthzTupleCapture(t, &call, true)
	defer srv.Close()

	auth := &middleware.AuthClient{Address: srv.URL, Enabled: true}

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

	handler := &AccountBlockExceptionHandler{
		Command: &command.UseCase{AccountRepo: accountRepo, TransactionRedisRepo: redisRepo},
	}

	f, path := mountBlockExceptionRoute(t, auth, handler)

	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(blockExceptionBody))
	req.Header.Set("Authorization", "Bearer "+guardBearerToken(t))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode, "a granted caller must get 201")

	assert.Equal(t, accountBlockExceptionResource, call.resource)

	var body mmodel.AccountBlockExceptions
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Exceptions, 1)

	assert.Equal(t, alias, body.Exceptions[0].AccountAlias)
	assert.Equal(t, "150.00", body.Exceptions[0].Amount)
	assert.NotEmpty(t, body.Exceptions[0].AccountBlockExceptionID)
	assert.False(t, body.Exceptions[0].ExpiresAt.IsZero(), "the response must carry the derived expiry")
}

// TestRegisterAccountBlockExceptionRoutes_PublishedOnV2Only locks the mount
// decision: the resource is new, so it is published on /v2 alone. A /v1 twin
// would be a deprecated copy of a route that never shipped.
func TestRegisterAccountBlockExceptionRoutes_PublishedOnV2Only(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/block-exceptions"

	item, ok := paths["/v2"+opPath]
	require.True(t, ok, "the /v2 surface must publish the block-exception create op")

	operation := operationForMethod(item, http.MethodPost)
	require.NotNil(t, operation, "the op must be a POST")
	assert.Equal(t, "createAccountBlockExceptions"+v2OpSuffix, operation.OperationID)

	_, onV1 := paths["/v1"+opPath]
	assert.False(t, onV1, "the block-exception surface must NOT be published on /v1")

	for _, unexpected := range []*huma.Operation{item.Get, item.Put, item.Patch, item.Delete, item.Head} {
		assert.Nil(t, unexpected, "the surface is write-only: POST is the only operation")
	}
}
