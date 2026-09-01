// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// This file guards the DEDICATED account block/unblock HTTP surface: two POST
// terminals per version group, both governed by a single authz resource of their
// own ("account-blocks", "post") rather than the ("accounts", "patch") tuple the
// generic account update carries. The separation is the whole point of the
// endpoints — an operator may be granted the power to freeze an account without
// being granted the power to rewrite it — so the authz tuple is asserted against
// a capturing authz server driven through the PRODUCTION registrars, never
// against a shim.
//
// The block ops are NOT holds of funds. /transactions/block and
// /transactions/unblock are a distinct, pre-existing concept on the transaction
// surface; nothing here touches them.

// accountBlockOps enumerates the two dedicated block-state operations and the
// operationId each publishes on /v1. Both are POST, both hang off the account-by-id
// path, and both answer with the account in the same shape the GET/PATCH ops use —
// which is what makes them subject to the same holder split (AccountV1 on /v1, the
// canonical Account on /v2).
var accountBlockOps = []struct {
	action        string
	opPath        string
	v1OperationID string
}{
	{
		action:        "block",
		opPath:        "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}/block",
		v1OperationID: "blockAccount",
	},
	{
		action:        "unblock",
		opPath:        "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}/unblock",
		v1OperationID: "unblockAccount",
	},
}

// accountBlockAuthzResource is the authz resource governing BOTH directions. One
// permission covers block and unblock on purpose: an operator who can freeze an
// account must be able to release it, and splitting the two would strand accounts
// frozen by an operator whose grant was later narrowed.
//
// Spelled literally because it is deployed Access Manager policy — a rename here
// is a breaking change to tenant grants, not an internal refactor.
const (
	accountBlockAuthzResource = "account-blocks"
	accountBlockAuthzAction   = "post"
)

// TestAccountBlockRoutes_PublishedOnBothVersions asserts both block-state ops are
// published on the REAL unified document — the same huma.API the served contract
// and the committed dump come from — under /v1 and /v2, each advertising the v1
// operationId with its version suffix appended.
//
// The suffix is not cosmetic: huma.OpenAPI.AddOperation panics on a duplicate
// operationId, so a v2 twin registered under the v1 id takes the ledger down at
// boot. Building the unified document here is therefore also the boot check.
func TestAccountBlockRoutes_PublishedOnBothVersions(t *testing.T) {
	t.Parallel()

	var api huma.API

	require.NotPanics(t, func() {
		_, api = buildUnifiedHumaAPI()
	}, "assembling the unified contract with the block ops must not panic on a duplicate operationId")

	paths := api.OpenAPI().Paths

	for _, op := range accountBlockOps {
		for _, v := range []struct {
			prefix string
			suffix string
		}{
			{prefix: "/v1", suffix: v1OpSuffix},
			{prefix: "/v2", suffix: v2OpSuffix},
		} {
			t.Run(op.action+v.prefix, func(t *testing.T) {
				t.Parallel()

				key := v.prefix + op.opPath

				item, ok := paths[key]
				require.Truef(t, ok, "the %s surface must publish the account %s op at %q", v.prefix, op.action, key)

				operation := operationForMethod(item, http.MethodPost)
				require.NotNilf(t, operation, "%s %q must carry a POST operation", op.action, key)

				assert.Equalf(t, op.v1OperationID+v.suffix, operation.OperationID,
					"the %s account %s op must advertise the v1 id with the version suffix", v.prefix, op.action)
			})
		}
	}
}

// TestAccountBlockRoutes_SplitAccountBodyByVersion locks the block ops onto the SAME
// holder split every other account-bearing op carries: /v1 answers with the
// holder-withholding "Account" component and /v2 with the holder-bearing "AccountV2".
// A block op that answered with the canonical account on /v1 would leak holderId and
// holderCheckSkipped onto a contract that never carried them.
func TestAccountBlockRoutes_SplitAccountBodyByVersion(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range accountBlockOps {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v1Item, ok := paths["/v1"+op.opPath]
			require.Truef(t, ok, "the /v1 surface must publish the account %s op", op.action)

			v2Item, ok := paths["/v2"+op.opPath]
			require.Truef(t, ok, "the /v2 surface must publish the account %s op", op.action)

			v1Op := operationForMethod(v1Item, http.MethodPost)
			require.NotNilf(t, v1Op, "the v1 account %s op must carry a POST operation", op.action)

			v2Op := operationForMethod(v2Item, http.MethodPost)
			require.NotNilf(t, v2Op, "the v2 account %s op must carry a POST operation", op.action)

			_, v1Resp := accountOpBodyRefs(v1Op)
			_, v2Resp := accountOpBodyRefs(v2Op)

			assert.ElementsMatchf(t, []string{accountV1ComponentName}, v1Resp,
				"the v1 account %s op must answer with the holder-withholding Account component", op.action)
			assert.ElementsMatchf(t, []string{accountV2ComponentName}, v2Resp,
				"the v2 account %s op must answer with the holder-bearing AccountV2 component", op.action)
		})
	}
}

// authzTuple is the (product, resource, action) triple the auth middleware forwards
// to the authorization service before it reads the decision.
type authzTuple struct {
	product  string
	resource string
	action   string
}

// newAuthzTupleCapture returns an httptest server standing in for the authz service.
// It records the full forwarded tuple — not just the product, as newAuthzProductCapture
// does — because the claim under test is precisely that the block ops carry a resource
// of their OWN. It always denies, so the caller's chain stops at the 403 and no
// business terminal runs.
func newAuthzTupleCapture(t *testing.T, captured *authzTuple) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("authz capture: decode request body: %v", err)
		}

		*captured = authzTuple{
			product:  body["product"],
			resource: body["resource"],
			action:   body["action"],
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte(`{"authorized":false}`)); err != nil {
			t.Errorf("authz capture: write response: %v", err)
		}
	}))
}

// TestAuthz_AccountBlockRoutes_UseDedicatedResource is the RBAC contract of this task.
// Both block-state ops, on BOTH version groups, must authorize under
// (midaz, account-blocks, post) — never under the generic (accounts, …) tuple the rest
// of the surface uses. It drives the PRODUCTION registrars through a capturing authz
// server, so a guard chain that forgot the block paths (leaving them mounted by the
// Huma terminal alone) never reaches the server and fails on the missing 403.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools; concurrent builds cross-contaminate.
func TestAuthz_AccountBlockRoutes_UseDedicatedResource(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	cases := []struct {
		version  string
		register func(group fiber.Router, api huma.API, auth *middleware.AuthClient)
	}{
		{
			version: "/v1",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				RegisterAccountRoutesToApp(group, api, auth, &AccountHandler{}, nil)
			},
		},
		{
			version: "/v2",
			register: func(group fiber.Router, api huma.API, auth *middleware.AuthClient) {
				RegisterAccountV2RoutesToApp(group, api, auth, &AccountHandler{}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			var captured authzTuple

			srv := newAuthzTupleCapture(t, &captured)
			defer srv.Close()

			auth := &middleware.AuthClient{Address: srv.URL, Enabled: true}

			f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
			libProblem.Install()
			f.Use(ledgerMiddleware.ErrorEnvelope())

			group := f.Group(tc.version)
			api := openapi.New(f, group, openapi.Config{Title: "authz-guard", Version: "test", Servers: []string{tc.version}})
			tc.register(group, api, auth)

			token := "Bearer " + guardBearerToken(t)
			base := tc.version + "/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() +
				"/accounts/" + accountID.String()

			for _, action := range []string{"block", "unblock"} {
				captured = authzTuple{}

				req := httptest.NewRequest(fiber.MethodPost, base+"/"+action, nil)
				req.Header.Set("Authorization", token)

				resp, err := f.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)
				require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
					"POST %s must reach auth and be denied; a mounted-but-unguarded route never would", base+"/"+action)

				assert.Equalf(t, midazName, captured.product,
					"the %s op must authorize under the midaz appName", action)
				assert.Equalf(t, accountBlockAuthzResource, captured.resource,
					"the %s op must authorize under its own dedicated resource", action)
				assert.Equalf(t, accountBlockAuthzAction, captured.action,
					"the %s op must authorize under the post action", action)
			}
		})
	}
}

// --- handler-level behaviour ---------------------------------------------------

// accountBlockRepos are the repository mocks the block state transition drives:
// the account read + source-of-truth write, the balance projection UPDATE, and the
// atomic multi-key cache DEL.
type accountBlockRepos struct {
	accountRepo *account.MockRepository
	balanceRepo *balance.MockRepository
	redisRepo   *redis.MockRedisRepository
}

// newAccountBlockHandler builds an AccountHandler whose command use case is wired to
// mock repositories, so the HTTP shells are exercised over the REAL command rather
// than a stubbed seam.
func newAccountBlockHandler(t *testing.T) (*AccountHandler, *accountBlockRepos) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repos := &accountBlockRepos{
		accountRepo: account.NewMockRepository(ctrl),
		balanceRepo: balance.NewMockRepository(ctrl),
		redisRepo:   redis.NewMockRedisRepository(ctrl),
	}

	handler := &AccountHandler{
		Command: &command.UseCase{
			AccountRepo:          repos.accountRepo,
			BalanceRepo:          repos.balanceRepo,
			TransactionRedisRepo: repos.redisRepo,
			Streaming:            pkgStreaming.NewMockEmitter(),
		},
	}

	return handler, repos
}

// buildHumaAccountBlockApp mounts ONE version group of the account surface, faithfully
// mirroring the production wiring: problem.Install before any huma.Register, the Huma
// API over the version group, ParseUUIDPathParameters as a Fiber middleware on the two
// block paths, then the version's Huma terminals.
//
// Only one version is mounted per app because the two contracts share operation IDs
// apart from the suffix and a single API cannot carry both without the suffix doing
// the work TestAccountBlockRoutes_PublishedOnBothVersions already proves.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func buildHumaAccountBlockApp(t *testing.T, handler *AccountHandler, version string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	f.Use(ledgerMiddleware.ErrorEnvelope())

	group := f.Group(version)
	api := openapi.New(f, group, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{version}})

	parse := pkgHTTP.ParseUUIDPathParameters("account")
	base := "/organizations/:organization_id/ledgers/:ledger_id/accounts/:id"
	group.Post(base+"/block", parse)
	group.Post(base+"/unblock", parse)

	if version == "/v2" {
		RegisterAccountV2Routes(api, handler, v2OpSuffix)
	} else {
		RegisterAccountRoutes(api, handler, v1OpSuffix)
	}

	return f
}

// blockableAccount is the pre-state AccountRepo.Find returns: a plain deposit account
// whose block column already holds the OPPOSITE of the requested state, so the
// transition is a real write rather than the idempotent short-circuit.
func blockableAccount(orgID, ledgerID, accountID uuid.UUID, blocked bool) *mmodel.Account {
	current := blocked

	return &mmodel.Account{
		ID:             accountID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Name:           "Blockable Account",
		Type:           "deposit",
		AssetCode:      "USD",
		Blocked:        &current,
		UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// expectBlockStateTransition arms the full happy-path sequence for a state change to
// target, asserting the holder policy the shell threaded into the command reaches the
// account read — that is how the /v1 and /v2 shells are told apart at the repository.
func expectBlockStateTransition(t *testing.T, repos *accountBlockRepos, orgID, ledgerID, accountID uuid.UUID, target bool, policy mmodel.HolderPolicy) {
	t.Helper()

	repos.accountRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, nil, accountID, policy).
		Return(blockableAccount(orgID, ledgerID, accountID, !target), nil).
		Times(1)

	repos.accountRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, nil, accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			require.NotNil(t, acc.Blocked, "Update must carry the new block state")
			assert.Equal(t, target, *acc.Blocked, "Update must carry the requested block state")

			out := *acc
			out.UpdatedAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

			return &out, nil
		}).
		Times(1)

	repos.balanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), orgID, ledgerID, accountID, target).
		Return(nil).
		Times(1)

	repos.balanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), orgID, ledgerID, accountID).
		Return([]*mmodel.Balance{{ID: uuid.New().String(), AccountID: accountID.String(), Alias: "@blockable", Key: "default"}}, nil).
		Times(1)

	repos.redisRepo.EXPECT().
		DelMany(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)
}

// TestAccountBlockEndpoints_HappyPath drives both directions on both version groups
// and asserts the endpoint answers 200 with the UPDATED account — the same body shape
// the GET/PATCH ops return — carrying the new block state. The /v1 cases additionally
// prove the holder keys stay withheld, since block/unblock reuse the account
// projection rather than minting a body of their own.
func TestAccountBlockEndpoints_HappyPath(t *testing.T) {
	// NOT parallel: process-global huma state.
	cases := []struct {
		name    string
		version string
		action  string
		target  bool
		policy  mmodel.HolderPolicy
	}{
		{name: "v1 block", version: "/v1", action: "block", target: true, policy: mmodel.HolderOffV1},
		{name: "v1 unblock", version: "/v1", action: "unblock", target: false, policy: mmodel.HolderOffV1},
		{name: "v2 block", version: "/v2", action: "block", target: true, policy: mmodel.HolderOnV2},
		{name: "v2 unblock", version: "/v2", action: "unblock", target: false, policy: mmodel.HolderOnV2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			accountID := uuid.Must(libCommons.GenerateUUIDv7())

			handler, repos := newAccountBlockHandler(t)
			expectBlockStateTransition(t, repos, orgID, ledgerID, accountID, tc.target, tc.policy)

			app := buildHumaAccountBlockApp(t, handler, tc.version)

			req := httptest.NewRequest(http.MethodPost, tc.version+"/organizations/"+orgID.String()+
				"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/"+tc.action, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			respBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(respBody))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))

			assert.Equal(t, accountID.String(), got["id"])
			assert.Equal(t, tc.target, got["blocked"], "the response must carry the new block state")
			assert.Equal(t, "Blockable Account", got["name"], "untouched account fields must survive")

			if tc.version == "/v1" {
				for _, key := range accountHolderKeys {
					assert.NotContainsf(t, got, key, "the /v1 block response must not advertise %q", key)
				}
			}
		})
	}
}

// TestAccountBlockEndpoints_NotFound_Canonical404 asserts a block-state change on an
// account that does not exist reaches the client as the canonical 0052 envelope, not
// a Huma-shaped error and not a 5xx.
func TestAccountBlockEndpoints_NotFound_Canonical404(t *testing.T) {
	// NOT parallel: process-global huma state.
	for _, action := range []string{"block", "unblock"} {
		t.Run(action, func(t *testing.T) {
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			accountID := uuid.Must(libCommons.GenerateUUIDv7())

			handler, repos := newAccountBlockHandler(t)

			repos.accountRepo.EXPECT().
				Find(gomock.Any(), orgID, ledgerID, nil, accountID, gomock.Any()).
				Return(nil, nil).
				Times(1)

			app := buildHumaAccountBlockApp(t, handler, "/v1")

			req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+
				"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/"+action, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			respBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			assert.Equal(t, http.StatusNotFound, resp.StatusCode, "body: %s", string(respBody))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
			assert.Equal(t, cn.ErrAccountIDNotFound.Error(), got["code"])
		})
	}
}

// TestAccountBlockEndpoints_ExternalAccount_Canonical422 asserts the external-account
// guard (0074) fires on the block surface exactly as it does on update and delete.
// External accounts anchor value crossing the ledger boundary; freezing one would
// strand every transaction that settles through it, so nothing may be written.
//
// 422, not 403: the ledger's canonical taxonomy maps 0074 to Unprocessable Entity on
// every command path that carries it, and 403 on this surface belongs to the authz
// chain (see TestAuthz_AccountBlockRoutes_UseDedicatedResource). Asserting the code
// alongside the status is what keeps the two rejections from being confused.
func TestAccountBlockEndpoints_ExternalAccount_Canonical422(t *testing.T) {
	// NOT parallel: process-global huma state.
	for _, action := range []string{"block", "unblock"} {
		t.Run(action, func(t *testing.T) {
			orgID := uuid.Must(libCommons.GenerateUUIDv7())
			ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
			accountID := uuid.Must(libCommons.GenerateUUIDv7())

			handler, repos := newAccountBlockHandler(t)

			external := blockableAccount(orgID, ledgerID, accountID, false)
			external.Type = "external"

			// Find is the ONLY repository call allowed: the guard runs before any write.
			repos.accountRepo.EXPECT().
				Find(gomock.Any(), orgID, ledgerID, nil, accountID, gomock.Any()).
				Return(external, nil).
				Times(1)

			app := buildHumaAccountBlockApp(t, handler, "/v2")

			req := httptest.NewRequest(http.MethodPost, "/v2/organizations/"+orgID.String()+
				"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/"+action, nil)

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			respBody, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "body: %s", string(respBody))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
			assert.Equal(t, cn.ErrForbiddenExternalAccountManipulation.Error(), got["code"])
		})
	}
}

// TestAccountBlockEndpoints_ExternalGuardIsPreWrite is a redundancy guard on the
// forbidden path that a status assertion alone cannot make: gomock fails the test if
// Update, the balance propagation or the cache DEL is called, so the 403 provably
// leaves the read models untouched. It is expressed as a separate test because the
// claim is about the calls that MUST NOT happen.
func TestAccountBlockEndpoints_ExternalGuardIsPreWrite(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	handler, repos := newAccountBlockHandler(t)

	external := blockableAccount(orgID, ledgerID, accountID, false)
	external.Type = "external"

	repos.accountRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, nil, accountID, gomock.Any()).
		Return(external, nil).
		Times(1)
	repos.accountRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	repos.balanceRepo.EXPECT().UpdateAccountBlockedByAccountID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	repos.redisRepo.EXPECT().DelMany(gomock.Any(), gomock.Any()).Times(0)

	app := buildHumaAccountBlockApp(t, handler, "/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+orgID.String()+
		"/ledgers/"+ledgerID.String()+"/accounts/"+accountID.String()+"/block", nil)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// TestAccountBlockRouteChain_GuardsBothPaths proves the Fiber guard chain — not just
// the Huma terminal — is attached to both block paths. A route Fiber holds only ONE
// entry for is a route mounted by the terminal alone, i.e. a public write endpoint.
// The block paths must carry the same two entries every other account op carries.
func TestAccountBlockRouteChain_GuardsBothPaths(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := fiber.New()

	libProblem.Install()

	group := app.Group("/v1")
	api := openapi.New(app, group, openapi.Config{Title: "chain", Version: "test", Servers: []string{"/v1"}})

	RegisterAccountRoutesToApp(group, api, &middleware.AuthClient{Enabled: false}, &AccountHandler{}, nil)

	shape := routeShapeOf(app, "/v1")

	for _, action := range []string{"block", "unblock"} {
		key := fiber.MethodPost + ":/organizations/:organization_id/ledgers/:ledger_id/accounts/:id/" + action

		entries, ok := shape[key]
		require.Truef(t, ok, "the account %s path must be mounted; mounted: %v", action, shape)
		assert.Lenf(t, entries, 2,
			"the account %s path must carry BOTH the guard chain and the Huma terminal, got %v", action, entries)
	}
}

// TestAccountBlockShells_RejectMalformedPathParams covers the defensive parse branches
// in the shells. Over HTTP those branches are unreachable — ParseUUIDPathParameters
// rejects a malformed id in the Fiber guard chain before the Huma terminal runs — which
// is precisely why they are exercised directly here: a shell that dropped its own parse
// check would still pass every request-level test in this file while handing a ZERO UUID
// to the command, and a zero UUID reads as a legitimate account id at the repository.
//
// The handler carries a nil Command on purpose: reaching it would be the failure.
func TestAccountBlockShells_RejectMalformedPathParams(t *testing.T) {
	t.Parallel()

	handler := &AccountHandler{}
	valid := uuid.New().String()

	requests := map[string]*AccountBlockRequest{
		"malformed organization id": {OrganizationID: "not-a-uuid", LedgerID: valid, ID: valid},
		"malformed ledger id":       {OrganizationID: valid, LedgerID: "not-a-uuid", ID: valid},
		"malformed account id":      {OrganizationID: valid, LedgerID: valid, ID: "not-a-uuid"},
	}

	for name, in := range requests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.BlockAccount(t.Context(), in)
			require.Error(t, err, "BlockAccount must reject %s before calling the command", name)

			_, err = handler.UnblockAccount(t.Context(), in)
			require.Error(t, err, "UnblockAccount must reject %s before calling the command", name)

			_, err = handler.BlockAccountV2(t.Context(), in)
			require.Error(t, err, "BlockAccountV2 must reject %s before calling the command", name)

			_, err = handler.UnblockAccountV2(t.Context(), in)
			require.Error(t, err, "UnblockAccountV2 must reject %s before calling the command", name)
		})
	}
}
