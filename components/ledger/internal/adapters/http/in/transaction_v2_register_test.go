// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const directV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/direct"

const holdV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/hold"

const blockV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/block"

const unblockV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/unblock"

const commitV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit"

const cancelV2RoutePath = "/v2/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/cancel"

// registerV2TransactionRoutesForTest wires the v2 transaction ops (direct AND hold) onto a
// fresh Fiber app + its own /v2 Huma contract, exactly as the production humaMountV2 seam
// does. A zero-value TransactionHandler is safe because registration never invokes the
// handler.
func registerV2TransactionRoutesForTest(auth *middleware.AuthClient) *fiber.App {
	app := fiber.New()

	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2RoutesToApp(apiV2, humaAPI, auth, &TransactionHandler{}, nil)

	return app
}

// TestRegisterTransactionV2RoutesToApp_MountsDirectRoute asserts the v2 `direct`
// POST route is mounted on the /v2 group (Fiber chain) and registered on the v2
// Huma contract with OperationID createTransactionDirectV2.
func TestRegisterTransactionV2RoutesToApp_MountsDirectRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerV2TransactionRoutesForTest(auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[http.MethodPost+":"+directV2RoutePath],
		"should register POST /v2 transactions/direct")
}

// TestRegisterTransactionV2RoutesToApp_MountsHoldRoute asserts the v2 `hold` POST
// route is mounted on the /v2 group (Fiber chain), sharing the direct/v1 protected
// chain.
func TestRegisterTransactionV2RoutesToApp_MountsHoldRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerV2TransactionRoutesForTest(auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[http.MethodPost+":"+holdV2RoutePath],
		"should register POST /v2 transactions/hold")
}

// TestRegisterTransactionV2RoutesToApp_MountsBlockRoute asserts the v2 `block` POST
// route is mounted on the /v2 group (Fiber chain), sharing the direct/hold/v1
// protected chain.
func TestRegisterTransactionV2RoutesToApp_MountsBlockRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerV2TransactionRoutesForTest(auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[http.MethodPost+":"+blockV2RoutePath],
		"should register POST /v2 transactions/block")
}

// TestRegisterTransactionV2RoutesToApp_MountsUnblockRoute asserts the v2 `unblock`
// POST route is mounted on the /v2 group (Fiber chain), sharing the
// direct/hold/v1 protected chain.
func TestRegisterTransactionV2RoutesToApp_MountsUnblockRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerV2TransactionRoutesForTest(auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[http.MethodPost+":"+unblockV2RoutePath],
		"should register POST /v2 transactions/unblock")
}

// TestRegisterTransactionV2RoutesToApp_MountsCommitRoute asserts the v2 `commit`
// lifecycle POST route (by transaction_id) is mounted on the /v2 group (Fiber
// chain), sharing the transactions:post protected chain.
func TestRegisterTransactionV2RoutesToApp_MountsCommitRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerV2TransactionRoutesForTest(auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[http.MethodPost+":"+commitV2RoutePath],
		"should register POST /v2 transactions/{transaction_id}/commit")
}

// TestRegisterTransactionV2RoutesToApp_MountsCancelRoute asserts the v2 `cancel`
// lifecycle POST route (by transaction_id) is mounted on the /v2 group (Fiber
// chain), sharing the transactions:post protected chain.
func TestRegisterTransactionV2RoutesToApp_MountsCancelRoute(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}
	app := registerV2TransactionRoutesForTest(auth)

	routeSet := make(map[string]bool)
	for _, r := range app.GetRoutes() {
		routeSet[r.Method+":"+r.Path] = true
	}

	assert.True(t, routeSet[http.MethodPost+":"+cancelV2RoutePath],
		"should register POST /v2 transactions/{transaction_id}/cancel")
}

// TestRegisterTransactionV2Routes_RegistersCommitHumaOperation asserts the commit op
// is present on the v2 Huma document with the canonical OperationID at the
// group-relative by-id commit path, defaulting to 201.
func TestRegisterTransactionV2Routes_RegistersCommitHumaOperation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit"

	pathItem, ok := humaAPI.OpenAPI().Paths[opPath]
	require.Truef(t, ok, "v2 contract should carry the commit op path %q", opPath)
	require.NotNil(t, pathItem.Post, "commit op path should carry a POST operation")

	assert.Equal(t, "commitTransactionV2", pathItem.Post.OperationID,
		"commit op should advertise OperationID commitTransactionV2")
	assert.Equal(t, http.StatusCreated, pathItem.Post.DefaultStatus,
		"commit op should default to 201 Created (lifecycle parity)")
}

// TestRegisterTransactionV2Routes_RegistersCancelHumaOperation asserts the cancel op
// is present on the v2 Huma document with the canonical OperationID at the
// group-relative by-id cancel path, defaulting to 201.
func TestRegisterTransactionV2Routes_RegistersCancelHumaOperation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel"

	pathItem, ok := humaAPI.OpenAPI().Paths[opPath]
	require.Truef(t, ok, "v2 contract should carry the cancel op path %q", opPath)
	require.NotNil(t, pathItem.Post, "cancel op path should carry a POST operation")

	assert.Equal(t, "cancelTransactionV2", pathItem.Post.OperationID,
		"cancel op should advertise OperationID cancelTransactionV2")
	assert.Equal(t, http.StatusCreated, pathItem.Post.DefaultStatus,
		"cancel op should default to 201 Created (lifecycle parity)")
}

// TestV2CommitRoute_RequiresAuth proves the v2 commit route shares the v1 protected
// chain: with auth enabled and no bearer token the request is rejected before
// reaching the stub handler.
func TestV2CommitRoute_RequiresAuth(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerV2TransactionRoutesForTest(auth)

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/00000000-0000-0000-0000-000000000003/commit"

	req := httptest.NewRequest(http.MethodPost, concretePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 commit request must be rejected by the transactions:post auth chain")
}

// TestV2CancelRoute_RequiresAuth proves the v2 cancel route shares the v1 protected
// chain: with auth enabled and no bearer token the request is rejected before
// reaching the stub handler.
func TestV2CancelRoute_RequiresAuth(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerV2TransactionRoutesForTest(auth)

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/00000000-0000-0000-0000-000000000003/cancel"

	req := httptest.NewRequest(http.MethodPost, concretePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 cancel request must be rejected by the transactions:post auth chain")
}

// TestRegisterTransactionV2Routes_RegistersBlockHumaOperation asserts the block op is
// present on the v2 Huma document with the canonical OperationID at the
// group-relative block path.
func TestRegisterTransactionV2Routes_RegistersBlockHumaOperation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/block"

	pathItem, ok := humaAPI.OpenAPI().Paths[opPath]
	require.Truef(t, ok, "v2 contract should carry the block op path %q", opPath)
	require.NotNil(t, pathItem.Post, "block op path should carry a POST operation")

	assert.Equal(t, "createTransactionBlockV2", pathItem.Post.OperationID,
		"block op should advertise OperationID createTransactionBlockV2")
	assert.Equal(t, http.StatusCreated, pathItem.Post.DefaultStatus,
		"block op should default to 201 Created (create parity)")
}

// TestRegisterTransactionV2Routes_RegistersUnblockHumaOperation asserts the unblock op
// is present on the v2 Huma document with the canonical OperationID at the
// group-relative unblock path.
func TestRegisterTransactionV2Routes_RegistersUnblockHumaOperation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/unblock"

	pathItem, ok := humaAPI.OpenAPI().Paths[opPath]
	require.Truef(t, ok, "v2 contract should carry the unblock op path %q", opPath)
	require.NotNil(t, pathItem.Post, "unblock op path should carry a POST operation")

	assert.Equal(t, "createTransactionUnblockV2", pathItem.Post.OperationID,
		"unblock op should advertise OperationID createTransactionUnblockV2")
	assert.Equal(t, http.StatusCreated, pathItem.Post.DefaultStatus,
		"unblock op should default to 201 Created (create parity)")
}

// TestV2BlockRoute_RequiresAuth proves the v2 block route shares the v1 protected chain:
// with auth enabled and no bearer token the request is rejected before reaching the
// stub handler.
func TestV2BlockRoute_RequiresAuth(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerV2TransactionRoutesForTest(auth)

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/block"

	req := httptest.NewRequest(http.MethodPost, concretePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 block request must be rejected by the transactions:post auth chain")
}

// TestV2UnblockRoute_RequiresAuth proves the v2 unblock route shares the v1 protected
// chain: with auth enabled and no bearer token the request is rejected before reaching
// the stub handler.
func TestV2UnblockRoute_RequiresAuth(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerV2TransactionRoutesForTest(auth)

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/unblock"

	req := httptest.NewRequest(http.MethodPost, concretePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 unblock request must be rejected by the transactions:post auth chain")
}

// TestRegisterTransactionV2Routes_RegistersHoldHumaOperation asserts the hold op is
// present on the v2 Huma document with the canonical OperationID at the
// group-relative hold path.
func TestRegisterTransactionV2Routes_RegistersHoldHumaOperation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/hold"

	pathItem, ok := humaAPI.OpenAPI().Paths[opPath]
	require.Truef(t, ok, "v2 contract should carry the hold op path %q", opPath)
	require.NotNil(t, pathItem.Post, "hold op path should carry a POST operation")

	assert.Equal(t, "createTransactionHoldV2", pathItem.Post.OperationID,
		"hold op should advertise OperationID createTransactionHoldV2")
	assert.Equal(t, http.StatusCreated, pathItem.Post.DefaultStatus,
		"hold op should default to 201 Created (create parity)")
}

// TestV2HoldRoute_RequiresAuth proves the v2 hold route shares the v1 protected chain:
// with auth enabled and no bearer token the request is rejected before reaching the
// stub handler.
func TestV2HoldRoute_RequiresAuth(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerV2TransactionRoutesForTest(auth)

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/hold"

	req := httptest.NewRequest(http.MethodPost, concretePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 hold request must be rejected by the transactions:post auth chain")
}

// TestRegisterTransactionV2Routes_RegistersHumaOperation asserts the direct op is
// present on the v2 Huma document with the canonical OperationID at the
// group-relative path.
func TestRegisterTransactionV2Routes_RegistersHumaOperation(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	apiV2 := app.Group("/v2")
	humaAPI := openapi.New(app, apiV2, openapi.Config{Title: "Midaz Ledger API v2", Version: "4.0.0", Servers: []string{"/v2"}})
	pkgHTTP.InstallLedgerSchemaNamer(humaAPI)

	RegisterTransactionV2Routes(humaAPI, &TransactionHandler{})

	const opPath = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/direct"

	pathItem, ok := humaAPI.OpenAPI().Paths[opPath]
	require.Truef(t, ok, "v2 contract should carry the direct op path %q", opPath)
	require.NotNil(t, pathItem.Post, "direct op path should carry a POST operation")

	assert.Equal(t, "createTransactionDirectV2", pathItem.Post.OperationID,
		"direct op should advertise OperationID createTransactionDirectV2")
	assert.Equal(t, http.StatusCreated, pathItem.Post.DefaultStatus,
		"direct op should default to 201 Created (create parity)")
}

// TestV2DirectRoute_RequiresAuth proves the v2 direct route shares the v1 protected
// chain: with auth enabled and no bearer token the request is rejected before
// reaching the stub handler.
func TestV2DirectRoute_RequiresAuth(t *testing.T) {
	t.Parallel()

	// Address must be non-empty so Authorize enforces the token check (it is never
	// dialed: a missing token short-circuits with 401 first).
	auth := &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"}
	app := registerV2TransactionRoutesForTest(auth)

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/direct"

	req := httptest.NewRequest(http.MethodPost, concretePath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 direct request must be rejected by the transactions:post auth chain")
}
