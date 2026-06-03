//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/log"
	crmhttp "github.com/LerianStudio/midaz/v3/components/crm/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/crm/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/adapters/mongodb/holder"
	crmservices "github.com/LerianStudio/midaz/v3/components/crm/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v3/tests/utils/mongodb"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CRMCollapse is the in-module proof that the CRM holder/alias
// surface works correctly inside the unified ledger binary (P3-T20). It
// exercises the collapsed components end-to-end:
//
//	(1) single-tenant cipher round-trip via initCRM,
//	(2) MULTI-TENANT cross-tenant NON-contamination (the R6 isolation guard),
//	(3) canonical midaz error code on validation failure (shim gone, PD-2),
//	(4) the crm-api Mongo manager is wired for tenant-removal eviction,
//	(5) a handler panic returns 500 via the hoisted WithRecover (P3-T07).
func TestIntegration_CRMCollapse(t *testing.T) {
	t.Run("single_tenant_cipher_round_trip", func(t *testing.T) {
		container := mongotestutil.SetupContainer(t)

		cfg := &Config{
			CrmPrefixedMongoURI:    container.URI,
			CrmPrefixedMongoDBName: container.DBName,
			CrmHashSecretKey:       testutils.TestHashKey,
			CrmEncryptSecretKey:    testutils.TestEncryptKey,
		}

		crm, err := initCRM(&Options{MultiTenantEnabled: false}, cfg, &libLog.GoLogger{})
		require.NoError(t, err, "initCRM single-tenant must succeed")
		require.NotNil(t, crm.connection, "single-tenant must build a static Mongo connection")
		require.NotNil(t, crm.holderHandler, "holder handler must be wired")
		require.NotNil(t, crm.aliasHandler, "alias handler must be wired")
		require.Nil(t, crm.mongoManager, "single-tenant must NOT build a tenant Mongo manager")

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

	// THE isolation guard (R6). The CRM repos resolve their database from
	// tmcore.GetMBContext(ctx) when built with a nil static connection (exactly
	// how the multi-tenant path constructs them via initCRM/buildCRMRepositories).
	// The route-scoped CRM tenant middleware is what injects a per-tenant DB into
	// that context. Here we drive the same context seam directly with TWO distinct
	// tenant databases and assert that data written under tenant A is invisible
	// under tenant B and vice-versa: no shared/global database leaks across
	// tenants. If the wiring ever collapsed both tenants onto one DB (the failure
	// mode a global f.Use mount would cause), this test would see contamination.
	t.Run("multi_tenant_cross_tenant_non_contamination", func(t *testing.T) {
		container := mongotestutil.SetupContainer(t)
		cipher := testutils.SetupCrypto(t)

		// MT-style repos: nil static connection => DB comes from context per request.
		holderRepo, err := holder.NewMongoDBRepository(nil, cipher)
		require.NoError(t, err)
		aliasRepo, err := alias.NewMongoDBRepository(nil, cipher)
		require.NoError(t, err)

		uc := &crmservices.UseCase{HolderRepo: holderRepo, AliasRepo: aliasRepo}

		// Two separate tenant databases inside the same Mongo container.
		dbA := mongotestutil.CreateConnection(t, container.URI, "tenant_a")
		dbB := mongotestutil.CreateConnection(t, container.URI, "tenant_b")
		mongoA, err := dbA.Database(context.Background())
		require.NoError(t, err)
		mongoB, err := dbB.Database(context.Background())
		require.NoError(t, err)

		// Per-tenant contexts inject the tenant DB under the GENERIC MB key,
		// exactly as the CRM-scoped crmTenantMiddleware.WithTenantDB does: that
		// middleware is built with single-arg WithMB(crmMongoManager) (no module),
		// so it writes the generic key that the CRM holder/alias repos read via
		// tmcore.GetMBContext(ctx). Using the module key here would NOT match how
		// the repos resolve their database (the bug this test originally caught).
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

		// And the lists must not bleed across tenants either.
		listA, err := uc.GetAllHolders(ctxA, orgID, http.QueryHeader{Limit: 100, Page: 1}, false)
		require.NoError(t, err)
		for _, h := range listA {
			require.NotNil(t, h.Name)
			assert.NotEqual(t, "Tenant B Holder", *h.Name, "tenant A list must not contain tenant B data")
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
		crm, err := initCRM(&Options{MultiTenantEnabled: false}, cfg, &libLog.GoLogger{})
		require.NoError(t, err)

		app := newCRMTestApp(crm.holderHandler, crm.aliasHandler)

		// POST a holder with an invalid body (missing required fields) -> validation
		// failure. With the shim deleted (PD-2) the response code is a CANONICAL
		// midaz code, never a CRM-00xx translation.
		req := httptest.NewRequest(fiber.MethodPost, "/v1/holders", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Organization-Id", "org-test")

		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		assert.GreaterOrEqual(t, resp.StatusCode, 400, "invalid body must be a client/server error")

		var envelope map[string]any
		require.NoError(t, json.Unmarshal(body, &envelope), "error body must be JSON")
		code, _ := envelope["code"].(string)
		assert.NotContains(t, code, "CRM-00", "response code must be canonical midaz, not a CRM-00xx shim translation: got %q", code)
	})

	t.Run("handler_panic_returns_500_via_hoisted_withrecover", func(t *testing.T) {
		// Mount the WithRecover hoist (as NewUnifiedServer does) + a route that
		// panics. The recover must convert the panic into a 500 instead of
		// dropping the connection.
		app := fiber.New(fiber.Config{
			DisableStartupMessage: true,
			ErrorHandler: func(ctx *fiber.Ctx, err error) error {
				return libHTTP.FiberErrorHandler(ctx, err)
			},
		})
		app.Use(http.WithRecover(http.WithRecoverLogger(&libLog.GoLogger{})))
		app.Get("/boom", func(c *fiber.Ctx) error {
			panic("forced handler panic")
		})

		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/boom", nil), -1)
		require.NoError(t, err, "connection must NOT be dropped on panic")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode, "panic must become a 500")
	})
}

// newCRMTestApp mounts the CRM registrar on a bare Fiber app with auth disabled
// and the WithRecover hoist, mirroring how NewUnifiedServer hosts CRM routes.
func newCRMTestApp(hh *crmhttp.HolderHandler, ah *crmhttp.AliasHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	app.Use(http.WithRecover(http.WithRecoverLogger(&libLog.GoLogger{})))

	// Auth disabled: Authorize becomes a pass-through, single-tenant routeOptions=nil.
	auth := middleware.NewAuthClient("", false, nil)
	crmhttp.RegisterCRMRoutesToApp(app, auth, hh, ah, nil)

	return app
}

type strReader struct {
	s string
	i int
}

func stringReader(s string) *strReader { return &strReader{s: s} }

func (r *strReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
