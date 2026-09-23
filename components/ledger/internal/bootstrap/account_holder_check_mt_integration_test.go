//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegration_AccountHolderCheckMultiTenant drives the /v2 account create
// through the REAL buildUnifiedRouteSetup onboarding route options, the real
// account registrar and the real CreateAccount use case, against two tenants
// whose onboarding PostgreSQL, transaction PostgreSQL, onboarding Mongo and CRM
// Mongo are distinct databases.
//
// With the ledger setting accounting.requireHolder=true the create asserts the
// named holder exists in CRM. The account route's tenant middleware binds only the
// module-keyed ledger stores, while the CRM holder repository reads the GENERIC
// Mongo key, so the holder reader must resolve the tenant's CRM database itself.
// Without that, every such create is a 500 "no database connection available"
// instead of reaching the holder rule.
func TestIntegration_AccountHolderCheckMultiTenant(t *testing.T) {
	// The lib-commons clients reject plaintext URIs unless ALLOW_INSECURE_TLS=true;
	// the testcontainers speak plaintext.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	logger := &libLog.GoLogger{}

	mongoContainer := mongotestutil.SetupReusableContainer(t)

	tenantA := seedCompositionTenant(t, mongoContainer, "tenanta")
	tenantB := seedCompositionTenant(t, mongoContainer, "tenantb")

	tenants := map[string]*compositionTenant{
		tenantA.tenantID: tenantA,
		tenantB.tenantID: tenantB,
	}

	// The account route's tenant middleware also resolves the transaction Mongo, so
	// every tenant gets one on top of the composition stores.
	transactionMongo := map[string]*mongo.Database{
		tenantA.tenantID: mongotestutil.CreateOwnedDatabase(t, mongoContainer),
		tenantB.tenantID: mongotestutil.CreateOwnedDatabase(t, mongoContainer),
	}

	tmServer := newFakeTenantManagerAccountStores(t, mongoContainer, tenants, transactionMongo)
	defer tmServer.Close()

	tenantClient, err := tmclient.NewClient(tmServer.URL, logger,
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tenantClient.Close() })

	onboardingPGManager := tmpostgres.NewManager(tenantClient, constant.ModuleOnboarding,
		tmpostgres.WithModule(constant.ModuleOnboarding), tmpostgres.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingPGManager.Close(context.Background()) })

	transactionPGManager := tmpostgres.NewManager(tenantClient, constant.ModuleTransaction,
		tmpostgres.WithModule(constant.ModuleTransaction), tmpostgres.WithLogger(logger))
	t.Cleanup(func() { _ = transactionPGManager.Close(context.Background()) })

	onboardingMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleOnboarding,
		tmmongo.WithModule(constant.ModuleOnboarding), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingMongoManager.Close(context.Background()) })

	transactionMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleTransaction,
		tmmongo.WithModule(constant.ModuleTransaction), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = transactionMongoManager.Close(context.Background()) })

	crmMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleCRM,
		tmmongo.WithModule(constant.ModuleCRM), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = crmMongoManager.Close(context.Background()) })

	setup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		onboardingPGManager, transactionPGManager,
		onboardingMongoManager, transactionMongoManager, crmMongoManager, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, setup.onboardingRouteOptions, "onboarding route options must be built in multi-tenant mode")
	require.NotNil(t, setup.crmRouteOptions, "CRM route options must be built in multi-tenant mode")

	// MT-style CRM repositories: nil static connection, so the holder read can only
	// reach a tenant CRM database the request context carries.
	fieldEncryptor := cipherFieldEncryptor(t, testutils.SetupCrypto(t))
	holderRepo, err := holder.NewMongoDBRepository(nil, fieldEncryptor)
	require.NoError(t, err)
	instrumentRepo, err := instrument.NewMongoDBRepository(nil, fieldEncryptor)
	require.NoError(t, err)

	crmUC := &crmservices.UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo}

	// MT-style ledger repositories: nil static connection with requireTenant set.
	queryUC := &query.UseCase{
		LedgerRepo:             ledger.NewLedgerPostgreSQLRepository(nil, true),
		AccountRepo:            account.NewAccountPostgreSQLRepository(nil, true),
		OnboardingMetadataRepo: onbMongo.NewMetadataMongoDBRepository(nil),
	}

	commandUC := &command.UseCase{
		LedgerRepo:             ledger.NewLedgerPostgreSQLRepository(nil, true),
		AssetRepo:              asset.NewAssetPostgreSQLRepository(nil, true),
		AccountRepo:            account.NewAccountPostgreSQLRepository(nil, true),
		BalanceRepo:            balance.NewBalancePostgreSQLRepository(nil, false, true),
		OnboardingMetadataRepo: onbMongo.NewMetadataMongoDBRepository(nil),
		SettingsReader:         queryUC,
		HolderReader:           holderReaderAdapter{service: crmUC, crmTenantDB: crmMongoManager},
	}

	auth := middleware.NewAuthClient("", false, nil)

	crmApp := newAccountHolderCheckApp(t, logger)
	mountCRMHuma(crmApp, auth, &httpin.HolderHandler{Service: crmUC}, &httpin.InstrumentHandler{Service: crmUC},
		nil, nil, nil, setup.crmRouteOptions, setup.crmLedgerReadsRouteOptions, setup.crmHolderDeleteRouteOptions)

	accountApp := newAccountHolderCheckApp(t, logger)
	mountAccountV2Huma(accountApp, auth, &httpin.AccountHandler{Command: commandUC, Query: queryUC}, setup.onboardingRouteOptions)

	// Tenant A's seeded ledger enforces the holder gate; a second ledger keeps the
	// default settings.
	setRequireHolderSetting(t, tenantA, tenantA.ledgerID)

	permissiveLedgerID := postgrestestutil.CreateTestLedgerWithParams(t, tenantA.onboardingPG.DB, tenantA.orgID,
		postgrestestutil.LedgerParams{Name: "Permissive Ledger", Status: "ACTIVE"})
	postgrestestutil.CreateTestAsset(t, tenantA.onboardingPG.DB, tenantA.orgID, permissiveLedgerID, "USD")

	t.Run("require_holder_with_the_tenants_own_holder_is_201_and_links_it", func(t *testing.T) {
		holderID := createHolderHTTP(t, crmApp, tenantA.tenantID, tenantA.orgID.String(), "Tenant A Account Holder", "11111111111")

		body, status := postAccountV2HTTP(t, accountApp, tenantA, tenantA.ledgerID, &holderID)
		require.Equalf(t, stdhttp.StatusCreated, status,
			"the holder check must read the tenant's own CRM Mongo; body: %s", body)

		var created mmodel.Account
		require.NoError(t, json.Unmarshal([]byte(body), &created), "body: %s", body)
		require.NotNil(t, created.HolderID, "body: %s", body)
		assert.Equal(t, holderID, *created.HolderID)

		var persisted string
		require.NoError(t, tenantA.onboardingPG.DB.QueryRow(
			`SELECT holder_id FROM account WHERE id = $1`, created.ID,
		).Scan(&persisted), "the account must be persisted in tenant A's onboarding PostgreSQL")
		assert.Equal(t, holderID, persisted, "the persisted account must carry the validated holder")
	})

	t.Run("require_holder_with_another_tenants_holder_is_holder_not_found_not_500", func(t *testing.T) {
		// The holder is a real document under tenant A's organization id, but only in
		// tenant B's CRM Mongo. Under tenant A's JWT the check must read tenant A's CRM
		// database and find nothing.
		holderID := createHolderHTTP(t, crmApp, tenantB.tenantID, tenantA.orgID.String(), "Tenant B Foreign Holder", "22222222222")
		require.Equal(t, stdhttp.StatusOK, getHolderStatusHTTP(t, crmApp, tenantB.tenantID, tenantA.orgID.String(), holderID),
			"the holder must exist in tenant B's CRM Mongo for the isolation assertion to mean anything")

		body, status := postAccountV2HTTP(t, accountApp, tenantA, tenantA.ledgerID, &holderID)
		assertProblem(t, body, status, stdhttp.StatusNotFound, constant.ErrHolderNotFound, constant.EntityHolder)

		var accounts int
		require.NoError(t, tenantA.onboardingPG.DB.QueryRow(
			`SELECT count(*) FROM account WHERE holder_id = $1`, holderID,
		).Scan(&accounts))
		assert.Zero(t, accounts, "a rejected holder must persist no account")
	})

	t.Run("without_require_holder_the_create_is_201_without_a_holder_check", func(t *testing.T) {
		// An id no tenant's CRM carries: with the gate off it is linked as supplied.
		holderID := uuid.NewString()

		body, status := postAccountV2HTTP(t, accountApp, tenantA, permissiveLedgerID, &holderID)
		require.Equalf(t, stdhttp.StatusCreated, status, "body: %s", body)

		var created mmodel.Account
		require.NoError(t, json.Unmarshal([]byte(body), &created), "body: %s", body)
		require.NotNil(t, created.HolderID, "body: %s", body)
		assert.Equal(t, holderID, *created.HolderID)
	})
}

// newAccountHolderCheckApp builds a bare Fiber app with the error handler and the
// recover hoist NewUnifiedServer installs.
func newAccountHolderCheckApp(t *testing.T, logger libLog.Logger) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: func(ctx fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	app.Use(http.WithRecover(http.WithRecoverLogger(logger)))
	t.Cleanup(func() { _ = app.Shutdown() })

	return app
}

// mountAccountV2Huma mounts the REAL /v2 account registrar on a bare Fiber app:
// the Fiber guard chain from routeOptions runs before the Huma terminal, as in
// NewUnifiedServer.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError hook.
func mountAccountV2Huma(app *fiber.App, auth *middleware.AuthClient, ah *httpin.AccountHandler, routeOptions *http.ProtectedRouteOptions) {
	libProblem.Install()
	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "account-integration", Version: "test", Servers: []string{"/v2"}})
	http.InstallLedgerSchemaNamer(hAPI)

	httpin.RegisterAccountV2RoutesToApp(apiV2, hAPI, auth, ah, routeOptions)
}

// setRequireHolderSetting enables accounting.requireHolder on a ledger through the
// ledger repository's settings write, against the tenant's onboarding PostgreSQL.
func setRequireHolderSetting(t *testing.T, tn *compositionTenant, ledgerID uuid.UUID) {
	t.Helper()

	db := dbresolver.New(dbresolver.WithPrimaryDBs(tn.onboardingPG.DB))
	ctx := tmcore.ContextWithPG(context.Background(), db, constant.ModuleOnboarding)

	_, err := ledger.NewLedgerPostgreSQLRepository(nil, true).UpdateSettings(ctx, tn.orgID, ledgerID,
		map[string]any{"accounting": map[string]any{"requireHolder": true}})
	require.NoError(t, err, "tenant %s: failed to enable requireHolder", tn.tenantID)
}

// postAccountV2HTTP issues the /v2 account create for a tenant, authenticated by
// the JWT tenantId claim the trusted-auth assertion reads.
func postAccountV2HTTP(t *testing.T, app *fiber.App, tn *compositionTenant, ledgerID uuid.UUID, holderID *string) (string, int) {
	t.Helper()

	alias := tn.alias("holdercheck")

	payload, err := json.Marshal(mmodel.CreateAccountInput{
		Name:      "holder check account",
		AssetCode: "USD",
		Type:      "deposit",
		Alias:     &alias,
		HolderID:  holderID,
	})
	require.NoError(t, err)

	url := fmt.Sprintf("/v2/organizations/%s/ledgers/%s/accounts", tn.orgID, ledgerID)

	req := httptest.NewRequest(stdhttp.MethodPost, url, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+tn.authToken)

	return doCRMRequest(t, app, req)
}

// newFakeTenantManagerAccountStores serves, per tenant, the composition stores
// (onboarding PostgreSQL + Mongo, transaction PostgreSQL, CRM Mongo) plus a
// transaction Mongo, which is every store the account route's tenant middleware
// resolves.
func newFakeTenantManagerAccountStores(t *testing.T, mongoContainer *mongotestutil.ContainerResult, tenants map[string]*compositionTenant, transactionMongo map[string]*mongo.Database) *httptest.Server {
	t.Helper()

	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/v1/tenants/", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		// Path: /v1/tenants/{tenantID}/associations/{service}/connections
		parts := strings.FieldsFunc(r.URL.Path, func(c rune) bool { return c == '/' })
		if len(parts) < 5 || parts[0] != "v1" || parts[1] != "tenants" || parts[3] != "associations" {
			stdhttp.Error(w, "invalid path", stdhttp.StatusBadRequest)
			return
		}

		tn, ok := tenants[parts[2]]
		if !ok {
			stdhttp.Error(w, "tenant not found", stdhttp.StatusNotFound)
			return
		}

		config := &tmcore.TenantConfig{
			ID:         tn.tenantID,
			TenantSlug: tn.tenantID,
			Status:     "active",
			Databases: map[string]tmcore.DatabaseConfig{
				constant.ModuleOnboarding: {
					PostgreSQL: compositionPGConfig(t, tn.onboardingPG),
					MongoDB:    &tmcore.MongoDBConfig{URI: mongoContainer.URI, Database: tn.onboardingMongo.Name()},
				},
				constant.ModuleTransaction: {
					PostgreSQL: compositionPGConfig(t, tn.transactionPG),
					MongoDB:    &tmcore.MongoDBConfig{URI: mongoContainer.URI, Database: transactionMongo[tn.tenantID].Name()},
				},
				constant.ModuleCRM: {
					MongoDB: &tmcore.MongoDBConfig{URI: mongoContainer.URI, Database: tn.crmMongo.Name()},
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
