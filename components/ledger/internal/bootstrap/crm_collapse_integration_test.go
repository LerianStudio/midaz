//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	"github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/log"
	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/crypto"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CRMCollapse is the in-module proof that the CRM holder/instrument
// surface works correctly inside the unified ledger binary. It exercises the
// collapsed components end-to-end:
//
//	(1) single-tenant cipher round-trip via initCRM,
//	(2) MULTI-TENANT cross-tenant NON-contamination at the use-case context seam,
//	(3) canonical midaz error code on validation failure (no CRM-00xx shim),
//	(4) a handler panic in the real CRM route group returns a clean 500 via the
//	    hoisted WithRecover, with no panic-string/stack-frame leak,
//	(5) HTTP-level cross-tenant isolation through the REAL route-scoped CRM
//	    tenant middleware, driven by the JWT tenantId the middleware reads.
func TestIntegration_CRMCollapse(t *testing.T) {
	t.Run("single_tenant_cipher_round_trip", func(t *testing.T) {
		container := mongotestutil.SetupContainer(t)

		cfg := &Config{
			CrmPrefixedMongoURI:    container.URI,
			CrmPrefixedMongoDBName: container.DBName,
			CrmHashSecretKey:       testutils.TestHashKey,
			CrmEncryptSecretKey:    testutils.TestEncryptKey,
		}

		crm, err := initCRM(&Options{MultiTenantEnabled: false}, cfg, nil, &libLog.GoLogger{})
		require.NoError(t, err, "initCRM single-tenant must succeed")
		require.NotNil(t, crm.connection, "single-tenant must build a static Mongo connection")
		require.NotNil(t, crm.holderHandler, "holder handler must be wired")
		require.NotNil(t, crm.instrumentHandler, "instrument handler must be wired")
		require.Nil(t, crm.mongoManager, "single-tenant must NOT build a tenant Mongo manager")
		t.Cleanup(func() { _ = crm.connection.Close(context.Background()) })

		ctx := context.Background()
		orgID := "org-" + uuid.New().String()[:8]

		natural := "NATURAL_PERSON"
		created, err := crm.holderHandler.Service.CreateHolder(ctx, orgID, &mmodel.CreateHolderInput{
			Type:     &natural,
			Name:     "Galadriel Of Lothlorien",
			Document: "12345678901",
		})
		require.NoError(t, err, "CreateHolder must succeed")
		require.NotNil(t, created)

		// Read back: proves the carried LCRYPTO_* keys decrypt PII written with them.
		got, err := crm.holderHandler.Service.GetHolderByID(ctx, orgID, *created.ID, false)
		require.NoError(t, err, "GetHolderByID must succeed")
		require.NotNil(t, got.Name)
		assert.Equal(t, "Galadriel Of Lothlorien", *got.Name, "decrypted name must round-trip")
		require.NotNil(t, got.Document)
		assert.Equal(t, "12345678901", *got.Document, "decrypted document must round-trip")
	})

	// Use-case-level isolation guard. The CRM repos resolve their database from
	// tmcore.GetMBContext(ctx) when built with a nil static connection (exactly
	// how the multi-tenant path constructs them via initCRM/buildCRMRepositories).
	// Here we drive that context seam directly with TWO distinct tenant databases
	// and assert data written under tenant A is invisible under tenant B and
	// vice-versa. The HTTP-level test below proves the same property end-to-end
	// through the real route-scoped middleware; this one isolates the repo layer.
	t.Run("multi_tenant_cross_tenant_non_contamination", func(t *testing.T) {
		container := mongotestutil.SetupContainer(t)
		fieldEncryptor := cipherFieldEncryptor(t, testutils.SetupCrypto(t))

		// MT-style repos: nil static connection => DB comes from context per request.
		holderRepo, err := holder.NewMongoDBRepository(nil, fieldEncryptor)
		require.NoError(t, err)
		instrumentRepo, err := instrument.NewMongoDBRepository(nil, fieldEncryptor)
		require.NoError(t, err)

		uc := &crmservices.UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo}

		// Two separate tenant databases inside the same Mongo container.
		dbA := mongotestutil.CreateConnection(t, container.URI, "tenant_a")
		dbB := mongotestutil.CreateConnection(t, container.URI, "tenant_b")
		mongoA, err := dbA.Database(context.Background())
		require.NoError(t, err)
		mongoB, err := dbB.Database(context.Background())
		require.NoError(t, err)

		// Per-tenant contexts inject the tenant DB under the GENERIC MB key,
		// which is the key the CRM holder/instrument repos read via
		// tmcore.GetMBContext(ctx). The route-scoped crmTenantMiddleware writes
		// this same generic key (it is built with single-arg WithMB); the
		// HTTP-level test below proves that wiring, this one isolates the repo.
		ctxA := tmcore.ContextWithMB(context.Background(), mongoA)
		ctxB := tmcore.ContextWithMB(context.Background(), mongoB)

		orgID := "org-shared" // same org id in both tenants: only the DB differs

		natural := "NATURAL_PERSON"
		hA, err := uc.CreateHolder(ctxA, orgID, &mmodel.CreateHolderInput{Type: &natural, Name: "Tenant A Holder", Document: "11111111111"})
		require.NoError(t, err, "tenant A CreateHolder must succeed")
		hB, err := uc.CreateHolder(ctxB, orgID, &mmodel.CreateHolderInput{Type: &natural, Name: "Tenant B Holder", Document: "22222222222"})
		require.NoError(t, err, "tenant B CreateHolder must succeed")

		// Tenant A must see ONLY its own holder; tenant B's holder must be absent.
		gotAinA, err := uc.GetHolderByID(ctxA, orgID, *hA.ID, false)
		require.NoError(t, err, "tenant A must read its own holder")
		require.NotNil(t, gotAinA.Name)
		assert.Equal(t, "Tenant A Holder", *gotAinA.Name)

		_, err = uc.GetHolderByID(ctxA, orgID, *hB.ID, false)
		require.Error(t, err, "tenant A MUST NOT find tenant B's holder (no cross-tenant leak)")

		// Symmetric check from tenant B.
		gotBinB, err := uc.GetHolderByID(ctxB, orgID, *hB.ID, false)
		require.NoError(t, err, "tenant B must read its own holder")
		require.NotNil(t, gotBinB.Name)
		assert.Equal(t, "Tenant B Holder", *gotBinB.Name)

		_, err = uc.GetHolderByID(ctxB, orgID, *hA.ID, false)
		require.Error(t, err, "tenant B MUST NOT find tenant A's holder (no cross-tenant leak)")

		// And the lists must not bleed across tenants either — checked from BOTH
		// sides so a leak in either direction is caught.
		listA, err := uc.GetAllHolders(ctxA, orgID, http.QueryHeader{Limit: 100, Page: 1}, false)
		require.NoError(t, err)
		for _, h := range listA {
			require.NotNil(t, h.Name)
			assert.NotEqual(t, "Tenant B Holder", *h.Name, "tenant A list must not contain tenant B data")
		}

		listB, err := uc.GetAllHolders(ctxB, orgID, http.QueryHeader{Limit: 100, Page: 1}, false)
		require.NoError(t, err)
		for _, h := range listB {
			require.NotNil(t, h.Name)
			assert.NotEqual(t, "Tenant A Holder", *h.Name, "tenant B list must not contain tenant A data")
		}
	})

	t.Run("validation_error_returns_canonical_midaz_code_not_crm_shim", func(t *testing.T) {
		container := mongotestutil.SetupContainer(t)

		cfg := &Config{
			CrmPrefixedMongoURI:    container.URI,
			CrmPrefixedMongoDBName: container.DBName,
			CrmHashSecretKey:       testutils.TestHashKey,
			CrmEncryptSecretKey:    testutils.TestEncryptKey,
		}
		crm, err := initCRM(&Options{MultiTenantEnabled: false}, cfg, nil, &libLog.GoLogger{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = crm.connection.Close(context.Background()) })

		app := newCRMTestApp(crm.holderHandler, crm.instrumentHandler)

		// POST a holder with an empty body: all three required fields (type, name,
		// document) are missing. WithBody validates the struct and returns a 400
		// Bad Request with the canonical midaz "missing fields" code (0009) — never
		// a CRM-00xx translation, because the standalone CRM error transformer is
		// gone.
		req := httptest.NewRequest(fiber.MethodPost,
			"/v1/organizations/"+uuid.New().String()+"/holders", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "error response body must be readable")

		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode,
			"missing required fields must be a 400 Bad Request")

		var envelope map[string]any
		require.NoError(t, json.Unmarshal(body, &envelope), "error body must be JSON")
		code, _ := envelope["code"].(string)
		assert.Equal(t, constant.ErrMissingFieldsInRequest.Error(), code,
			"missing-fields validation must surface the canonical midaz code, got %q", code)
		assert.NotContains(t, code, "CRM-00",
			"response code must be canonical midaz, not a CRM-00xx shim translation: got %q", code)
	})

	t.Run("handler_panic_returns_500_via_hoisted_withrecover_no_leak", func(t *testing.T) {
		container := mongotestutil.SetupContainer(t)

		cfg := &Config{
			CrmPrefixedMongoURI:    container.URI,
			CrmPrefixedMongoDBName: container.DBName,
			CrmHashSecretKey:       testutils.TestHashKey,
			CrmEncryptSecretKey:    testutils.TestEncryptKey,
		}
		crm, err := initCRM(&Options{MultiTenantEnabled: false}, cfg, nil, &libLog.GoLogger{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = crm.connection.Close(context.Background()) })

		// Mount the real CRM route group under the hoisted WithRecover (as
		// NewUnifiedServer does). A post-auth middleware on the CRM routes panics,
		// so the panic fires INSIDE the real CRM route group's protected chain.
		// The hoisted recover must convert it into a 500 without dropping the
		// connection and without leaking the panic message or stack frames.
		const panicMessage = "forced CRM route panic"

		app := fiber.New(fiber.Config{
			DisableStartupMessage: true,
			ErrorHandler: func(ctx *fiber.Ctx, err error) error {
				return libHTTP.FiberErrorHandler(ctx, err)
			},
		})
		app.Use(http.WithRecover(http.WithRecoverLogger(&libLog.GoLogger{})))

		auth := middleware.NewAuthClient("", false, nil)
		panicOptions := &http.ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{
				func(c *fiber.Ctx) error { panic(panicMessage) },
			},
		}
		mountCRMHuma(app, auth, crm.holderHandler, crm.instrumentHandler, nil, crm.encryptionHandler, crm.auditHandler, panicOptions)

		req := httptest.NewRequest(fiber.MethodGet,
			"/v1/organizations/"+uuid.New().String()+"/holders/"+uuid.New().String(), nil)

		resp, err := app.Test(req, -1)
		require.NoError(t, err, "connection must NOT be dropped on panic")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode, "panic must become a 500")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "500 response body must be readable")

		assert.NotContains(t, string(body), panicMessage,
			"the 500 body must NOT leak the panic message")
		assert.NotContains(t, string(body), "goroutine ",
			"the 500 body must NOT leak goroutine stack frames")
		assert.NotContains(t, string(body), ".go:",
			"the 500 body must NOT leak source file/line stack frames")
	})

	// HTTP-level cross-tenant isolation through the REAL route-scoped CRM tenant
	// middleware. Two tenants resolve to two Mongo databases (same orgID, same
	// container) via the production seam: a JWT tenantId claim -> the
	// tenant-manager client -> the crm-api Mongo manager -> the generic MB
	// context key the CRM repos read. Tenant identity comes from the token the
	// middleware actually parses, NOT from a hand-injected tmcore.ContextWithMB.
	t.Run("http_cross_tenant_isolation_via_route_scoped_middleware", func(t *testing.T) {
		runHTTPCrossTenantIsolation(t, false)
	})
}

// runHTTPCrossTenantIsolation drives two tenants through the real route-scoped
// CRM tenant middleware over HTTP and asserts bidirectional not-found plus
// list-level zero-leak. When breakIsolation is true it collapses both tenants
// onto a single database, which is the mutation used to prove the test actually
// catches a broken isolation wiring (it must fail in that mode).
func runHTTPCrossTenantIsolation(t *testing.T, breakIsolation bool) {
	t.Helper()

	container := mongotestutil.SetupContainer(t)
	logger := &libLog.GoLogger{}

	const (
		tenantA = "tenant-a"
		tenantB = "tenant-b"
		dbA     = "crm_tenant_a"
		dbB     = "crm_tenant_b"
		// Identical org in both tenants: only the DB differs. A literal UUID
		// because organization_id is now a path parameter under UUID validation.
		orgID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	)

	// When isolation is broken, both tenants are pointed at the same database.
	tenantDBs := map[string]string{tenantA: dbA, tenantB: dbB}
	if breakIsolation {
		tenantDBs[tenantB] = dbA
	}

	// Fake tenant-manager control plane: returns a per-tenant crm-api MongoDB
	// config pointing at the shared container URI with a tenant-specific
	// database name. This is the seam the crm-api Mongo manager resolves.
	tmServer := newFakeTenantManagerMongo(t, container.URI, tenantDBs)
	defer tmServer.Close()

	tenantClient, err := tmclient.NewClient(tmServer.URL, logger,
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tenantClient.Close() })

	// Build the crm-api tenant Mongo manager exactly as initCRMMultiTenant does:
	// WithModule(crm-api), service name "crm-api", backed by the tenant client.
	crmMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleCRM,
		tmmongo.WithModule(constant.ModuleCRM), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = crmMongoManager.Close(context.Background()) })

	// Real CRM tenant middleware, constructed exactly as the ledger composition
	// root does: a SEPARATE instance carrying ONLY the crm-api manager, with
	// single-arg WithMB so it writes the generic MB context key the CRM repos
	// read. Cache + loader mirror production lazy-load behavior.
	tenantCache := tenantcache.NewTenantCache()
	tenantLoader := tenantcache.NewTenantLoader(tenantClient, tenantCache, constant.ModuleCRM, time.Minute, logger)
	crmTenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithMB(crmMongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	// MT repos: nil static connection => DB resolved from the request context the
	// middleware populates.
	fieldEncryptor := cipherFieldEncryptor(t, testutils.SetupCrypto(t))
	holderRepo, err := holder.NewMongoDBRepository(nil, fieldEncryptor)
	require.NoError(t, err)
	instrumentRepo, err := instrument.NewMongoDBRepository(nil, fieldEncryptor)
	require.NoError(t, err)
	useCases := &crmservices.UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo}
	holderHandler := &httpin.HolderHandler{Service: useCases}
	instrumentHandler := &httpin.InstrumentHandler{Service: useCases}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	app.Use(http.WithRecover(http.WithRecoverLogger(logger)))

	// Auth disabled (Authorize is a pass-through); the CRM-scoped route options
	// carry exactly the production post-auth chain: the trusted-auth assertion
	// (which seeds tenant id from the JWT) followed by the route-scoped CRM
	// tenant middleware that resolves the per-tenant Mongo DB.
	auth := middleware.NewAuthClient("", false, nil)
	crmRouteOptions := &http.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{
			http.MarkTrustedAuthAssertion(),
			crmTenantMiddleware.WithTenantDB,
		},
	}
	mountCRMHuma(app, auth, holderHandler, instrumentHandler, nil, nil, nil, crmRouteOptions)

	// Create one holder per tenant, addressing tenants ONLY via the JWT.
	idA := createHolderHTTP(t, app, tenantA, orgID, "Tenant A Holder", "11111111111")
	idB := createHolderHTTP(t, app, tenantB, orgID, "Tenant B Holder", "22222222222")

	// Each tenant reads its own holder.
	assert.Equal(t, fiber.StatusOK, getHolderStatusHTTP(t, app, tenantA, orgID, idA),
		"tenant A must read its own holder")
	assert.Equal(t, fiber.StatusOK, getHolderStatusHTTP(t, app, tenantB, orgID, idB),
		"tenant B must read its own holder")

	// Cross-tenant reads must be not-found in both directions.
	assert.Equal(t, fiber.StatusNotFound, getHolderStatusHTTP(t, app, tenantA, orgID, idB),
		"tenant A MUST NOT find tenant B's holder")
	assert.Equal(t, fiber.StatusNotFound, getHolderStatusHTTP(t, app, tenantB, orgID, idA),
		"tenant B MUST NOT find tenant A's holder")

	// List-level zero-leak: tenant A's list must not contain tenant B's holder.
	listA := listHolderNamesHTTP(t, app, tenantA, orgID)
	assert.NotContains(t, listA, "Tenant B Holder", "tenant A list must not leak tenant B data")
	assert.Contains(t, listA, "Tenant A Holder", "tenant A list must contain its own holder")
}

// mountCRMHuma wires the Huma-migrated CRM registrar on app, mirroring the
// production humaMount seam: problem.Install() before any huma.Register, the shared
// Huma API built with openapi.New over a /v1 group, and RegisterCRMRoutesToApp
// attaching the Fiber auth+tenant middleware chain plus the Huma terminals on that
// group. The middleware chain (auth + routeOptions PostAuthMiddlewares +
// ParseUUIDPathParameters) runs BEFORE each Huma terminal, exactly as in the unified
// server, so these integration tests exercise the real request path end-to-end.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError
// hook and Huma validation uses process-global sync.Pools.
func mountCRMHuma(app *fiber.App, auth *middleware.AuthClient, hh *httpin.HolderHandler, ah *httpin.InstrumentHandler, hah *httpin.HolderAccountsHandler, eh *httpin.EncryptionHandler, auditHandler *httpin.AuditHandler, routeOptions *http.ProtectedRouteOptions) {
	libProblem.Install()
	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{Title: "crm-integration", Version: "test", Servers: []string{"/v1"}})
	http.InstallLedgerSchemaNamer(hAPI)

	httpin.RegisterCRMRoutesToApp(apiV1, hAPI, auth, hh, ah, hah, eh, auditHandler, routeOptions)
}

// newCRMTestApp mounts the CRM registrar on a bare Fiber app with auth disabled
// and the WithRecover hoist, mirroring how NewUnifiedServer hosts CRM routes.
func newCRMTestApp(hh *httpin.HolderHandler, ah *httpin.InstrumentHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	app.Use(http.WithRecover(http.WithRecoverLogger(&libLog.GoLogger{})))

	// Auth disabled: Authorize becomes a pass-through, single-tenant routeOptions=nil.
	auth := middleware.NewAuthClient("", false, nil)
	mountCRMHuma(app, auth, hh, ah, nil, nil, nil, nil)

	return app
}

// cipherFieldEncryptor wraps the test cipher in the legacy-mode FieldEncryptor
// the holder/instrument repositories require, reusing the same wiring the
// single-tenant/legacy bootstrap path uses (wireEncryptionServices + adapter).
func cipherFieldEncryptor(t *testing.T, cipher encryption.LegacyCrypto) encryption.FieldEncryptor {
	t.Helper()

	wired := wireEncryptionServices(wireEncryptionServicesInput{
		mode:         crypto.EncryptionModeLegacy.String(),
		legacyCrypto: cipher,
	})
	require.NoError(t, wired.err)

	return encryption.NewFieldEncryptorAdapter(wired.encryptionService)
}

// newFakeTenantManagerMongo returns a tenant-manager stub that serves the
// crm-api MongoDB config per tenant. The config points every tenant at the same
// container URI but a tenant-specific database name, so the manager resolves a
// distinct *mongo.Database per tenant.
func newFakeTenantManagerMongo(t *testing.T, mongoURI string, tenantDBs map[string]string) *httptest.Server {
	t.Helper()

	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/v1/tenants/", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		// Path: /v1/tenants/{tenantID}/associations/{service}/connections
		parts := strings.FieldsFunc(r.URL.Path, func(c rune) bool { return c == '/' })
		if len(parts) < 5 || parts[0] != "v1" || parts[1] != "tenants" || parts[3] != "associations" {
			stdhttp.Error(w, "invalid path", stdhttp.StatusBadRequest)
			return
		}

		tenantID := parts[2]

		dbName, ok := tenantDBs[tenantID]
		if !ok {
			stdhttp.Error(w, "tenant not found", stdhttp.StatusNotFound)
			return
		}

		config := &tmcore.TenantConfig{
			ID:         tenantID,
			TenantSlug: tenantID,
			Status:     "active",
			Databases: map[string]tmcore.DatabaseConfig{
				constant.ModuleCRM: {
					MongoDB: &tmcore.MongoDBConfig{
						URI:      mongoURI,
						Database: dbName,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(config); err != nil {
			stdhttp.Error(w, "encode failed", stdhttp.StatusInternalServerError)
		}
	})

	return httptest.NewServer(mux)
}

// tenantJWT builds an unsigned-but-parseable HS256 token carrying the tenantId
// claim the CRM tenant middleware reads via ParseUnverified. The signature is
// never verified by the middleware, so any key works.
func tenantJWT(t *testing.T, tenantID string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenantId": tenantID,
		"sub":      "test-user",
	})

	signed, err := token.SignedString([]byte("test-signing-key"))
	require.NoError(t, err, "failed to sign test JWT")

	return signed
}

func createHolderHTTP(t *testing.T, app *fiber.App, tenantID, orgID, name, document string) string {
	t.Helper()

	body, err := json.Marshal(mmodel.CreateHolderInput{
		Type:     testutils.Ptr("NATURAL_PERSON"),
		Name:     name,
		Document: document,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(fiber.MethodPost,
		"/v1/organizations/"+orgID+"/holders", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+tenantJWT(t, tenantID))

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode,
		"CreateHolder must succeed for tenant %s, got %d: %s", tenantID, resp.StatusCode, string(respBody))

	var created mmodel.Holder
	require.NoError(t, json.Unmarshal(respBody, &created))
	require.NotNil(t, created.ID)

	return created.ID.String()
}

func getHolderStatusHTTP(t *testing.T, app *fiber.App, tenantID, orgID, holderID string) int {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodGet,
		"/v1/organizations/"+orgID+"/holders/"+holderID, nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+tenantJWT(t, tenantID))

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode
}

func listHolderNamesHTTP(t *testing.T, app *fiber.App, tenantID, orgID string) []string {
	t.Helper()

	req := httptest.NewRequest(fiber.MethodGet,
		"/v1/organizations/"+orgID+"/holders?limit=100", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+tenantJWT(t, tenantID))

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.StatusCode,
		"GetAllHolders must succeed for tenant %s, got %d: %s", tenantID, resp.StatusCode, string(respBody))

	var page struct {
		Items []mmodel.Holder `json:"items"`
	}
	require.NoError(t, json.Unmarshal(respBody, &page))

	names := make([]string, 0, len(page.Items))
	for _, h := range page.Items {
		if h.Name != nil {
			names = append(names, *h.Name)
		}
	}

	return names
}
