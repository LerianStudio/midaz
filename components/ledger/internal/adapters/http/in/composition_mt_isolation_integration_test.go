//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/log"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	crmholder "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	crminstrument "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/LerianStudio/midaz/v4/tests/utils/stubs"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/sync/errgroup"
)

// compositionTenant holds the per-tenant scaffolding the isolation gate needs:
// a distinct onboarding-PG database (org + ledger + asset seeded) and a distinct
// CRM-Mongo database (holder seeded). The composition POST for this tenant must
// write the account ONLY into its PG and the instrument ONLY into its Mongo.
type compositionTenant struct {
	tenantID string

	// onboarding PG
	pgDBName string
	pgDB     *sql.DB // raw connection to this tenant's onboarding DB, for assertions
	orgID    uuid.UUID
	ledgerID uuid.UUID

	// CRM Mongo
	mongoDBName string
	holderID    uuid.UUID
}

// TestIntegration_CompositionConcurrentTenantIsolation is the R7 HARD GATE.
//
// It drives the REAL cross-store composition tenant middleware (the F4-T01
// instance: WithPG(onboardingPGManager, ModuleOnboarding) + bare
// WithMB(crmMongoManager)) over HTTP, with two tenants (A, B) each owning a
// distinct onboarding-PG database AND a distinct CRM-Mongo database, resolved
// per request from a fake tenant-manager via the JWT tenantId — exactly the
// production seam. It fires the composition POST /v1/holders/:id/accounts for
// both tenants CONCURRENTLY (errgroup), each with an instrument, and asserts:
//
//	(1) tenant A's account lands ONLY in A's onboarding PG, never in B's;
//	(2) tenant A's instrument lands ONLY in A's CRM Mongo, never in B's;
//	(3) symmetric for B;
//	(4) no cross-store key collision bleeds one tenant's CRM Mongo onto the
//	    other's PG-keyed onboarding context, or vice versa.
//
// Run with -race. If isolation fails, the bug is a key collision in the
// composition middleware (F4-T01): the generic Mongo key bleeding onto a route
// it should not, or the middleware mounted globally rather than route-scoped.
func TestIntegration_CompositionConcurrentTenantIsolation(t *testing.T) {
	// Both the direct setup connections (CreateConnection) and the per-tenant
	// connections the tmmongo manager opens lazily go through libMongo.NewClient,
	// which rejects plaintext URIs unless ALLOW_INSECURE_TLS=true. The
	// testcontainer Mongo speaks plaintext, so bypass the TLS gate for the test.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	logger := &libLog.GoLogger{}

	pgContainer := postgrestestutil.SetupContainer(t)
	mongoContainer := mongotestutil.SetupContainer(t)

	tenantA := newCompositionTenant(t, pgContainer, mongoContainer, "tenanta", "comp_onb_a", "comp_crm_a")
	tenantB := newCompositionTenant(t, pgContainer, mongoContainer, "tenantb", "comp_onb_b", "comp_crm_b")

	tenants := map[string]*compositionTenant{
		tenantA.tenantID: tenantA,
		tenantB.tenantID: tenantB,
	}

	// Fake tenant-manager control plane: per tenant it returns BOTH the
	// onboarding PostgreSQL config (module key "onboarding") AND the CRM MongoDB
	// config (module key "crm-api"), each pointing at the shared container with a
	// tenant-specific database name. The PG manager (module=onboarding) and the
	// Mongo manager (module=crm-api) each resolve their own slice of the same
	// per-tenant config blob. Resolution is keyed by module, not service, so one
	// blob serves both managers.
	tmServer := newFakeTenantManagerCrossStore(t, pgContainer, mongoContainer, tenants)
	defer tmServer.Close()

	tenantClient, err := tmclient.NewClient(tmServer.URL, logger,
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tenantClient.Close() })

	// Production-shaped managers: the onboarding PG manager is module-keyed
	// "onboarding"; the CRM Mongo manager is module-keyed "crm-api" for
	// tenant-manager DB resolution but its REQUEST-context key is generic (the
	// middleware below uses bare WithMB).
	onboardingPGManager := tmpostgres.NewManager(tenantClient, constant.ModuleOnboarding,
		tmpostgres.WithModule(constant.ModuleOnboarding), tmpostgres.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingPGManager.Close(context.Background()) })

	crmMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleCRM,
		tmmongo.WithModule(constant.ModuleCRM), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = crmMongoManager.Close(context.Background()) })

	tenantCache := tenantcache.NewTenantCache()
	tenantLoader := tenantcache.NewTenantLoader(tenantClient, tenantCache, constant.ModuleOnboarding, time.Minute, logger)

	// The EXACT composition tenant middleware the ledger composition root builds
	// (config.go:1308). Module-keyed WithPG for the onboarding account write,
	// bare WithMB for the generic-key CRM instrument write.
	compositionTenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithPG(onboardingPGManager, constant.ModuleOnboarding),
		tmmiddleware.WithMB(crmMongoManager),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	// Use cases. The onboarding account-create use case reads its PG connection
	// from the module-keyed ("onboarding") context; the CRM instrument-create use
	// case reads its Mongo database from the generic context key. Both repos are
	// built MT-style (nil static connection) so the DB comes from the request
	// context the middleware populates.
	accountRepo := account.NewAccountPostgreSQLRepository(nil, true)
	orgRepo := organization.NewOrganizationPostgreSQLRepository(nil, true)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(nil, true)
	assetRepo := asset.NewAssetPostgreSQLRepository(nil, true)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(nil, true)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(nil, true)
	onbMetadataRepo := mongodb.NewMetadataMongoDBRepository(nil)

	commandUC := &command.UseCase{
		OrganizationRepo:       orgRepo,
		LedgerRepo:             ledgerRepo,
		AssetRepo:              assetRepo,
		AccountRepo:            accountRepo,
		PortfolioRepo:          portfolioRepo,
		SegmentRepo:            segmentRepo,
		OnboardingMetadataRepo: onbMetadataRepo,
		BalanceRepo:            stubs.NewBalanceRepoStub(),
	}

	cipher := testutils.SetupCrypto(t)
	holderRepo, err := crmholder.NewMongoDBRepository(nil, cipher)
	require.NoError(t, err)
	instrumentRepo, err := crminstrument.NewMongoDBRepository(nil, cipher)
	require.NoError(t, err)
	crmUC := &crmservices.UseCase{
		HolderRepo:     holderRepo,
		InstrumentRepo: instrumentRepo,
		// Instrument referential validation reads LedgerAccounts; the composition
		// POSTs link a genuinely persisted ledger+account, so the both-exist stub
		// mirrors reality, matching the sibling composition tests.
		LedgerAccounts: stubInstrumentLedgerAccountReader{ledgerExists: true, accountExists: true},
	}

	compositionHandler := &CompositionHandler{Service: composition.NewService(commandUC, crmUC)}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})
	app.Use(midazhttp.WithRecover(midazhttp.WithRecoverLogger(logger)))
	t.Cleanup(func() { _ = app.Shutdown() })

	// Auth disabled (Authorize is a pass-through); the composition route options
	// carry EXACTLY the production post-auth chain: the trusted-auth assertion
	// (seeds tenant id from the JWT) followed by the route-scoped composition
	// tenant middleware that resolves BOTH per-tenant stores.
	auth := middleware.NewAuthClient("", false, nil)
	routeOptions := &midazhttp.ProtectedRouteOptions{
		PostAuthMiddlewares: []fiber.Handler{
			midazhttp.MarkTrustedAuthAssertion(),
			compositionTenantMiddleware.WithTenantDB,
		},
	}
	RegisterCompositionRoutesToApp(app, auth, compositionHandler, routeOptions)

	// Concurrent composition POSTs, one per tenant, each with an instrument.
	var g errgroup.Group

	for _, tn := range []*compositionTenant{tenantA, tenantB} {
		tn := tn
		g.Go(func() error {
			return postComposition(app, tn)
		})
	}

	require.NoError(t, g.Wait(), "concurrent composition POSTs must both succeed")

	// --- PG isolation: each tenant's account lands ONLY in its own onboarding DB ---
	accountsA := holderAccountAliases(t, tenantA.pgDB, tenantA.orgID, tenantA.ledgerID, tenantA.holderID)
	accountsB := holderAccountAliases(t, tenantB.pgDB, tenantB.orgID, tenantB.ledgerID, tenantB.holderID)

	require.Len(t, accountsA, 1, "tenant A must have exactly one account in its onboarding PG")
	require.Len(t, accountsB, 1, "tenant B must have exactly one account in its onboarding PG")
	assert.Equal(t, "@comp-tenanta", accountsA[0], "tenant A's account alias must be A's")
	assert.Equal(t, "@comp-tenantb", accountsB[0], "tenant B's account alias must be B's")

	// Cross-store leak guard (PG): tenant B's holder must own NO account in A's
	// PG and tenant A's holder must own NO account in B's PG.
	assert.Empty(t, holderAccountAliases(t, tenantA.pgDB, tenantA.orgID, tenantA.ledgerID, tenantB.holderID),
		"tenant B's holder must NOT own any account in tenant A's onboarding PG")
	assert.Empty(t, holderAccountAliases(t, tenantB.pgDB, tenantB.orgID, tenantB.ledgerID, tenantA.holderID),
		"tenant A's holder must NOT own any account in tenant B's onboarding PG")

	// And A's account alias must be wholly absent from B's PG (and vice versa).
	assert.Empty(t, accountAliasesByValue(t, tenantB.pgDB, "@comp-tenanta"),
		"tenant A's account alias must NOT appear anywhere in tenant B's onboarding PG")
	assert.Empty(t, accountAliasesByValue(t, tenantA.pgDB, "@comp-tenantb"),
		"tenant B's account alias must NOT appear anywhere in tenant A's onboarding PG")

	// --- Mongo isolation: each tenant's instrument lands ONLY in its own CRM DB ---
	mongoClient := mongoContainer.Client

	instrA := instrumentDocsForHolder(t, mongoClient, tenantA.mongoDBName, tenantA.orgID.String(), tenantA.holderID)
	instrB := instrumentDocsForHolder(t, mongoClient, tenantB.mongoDBName, tenantB.orgID.String(), tenantB.holderID)

	require.Equal(t, int64(1), instrA, "tenant A must have exactly one instrument in its CRM Mongo")
	require.Equal(t, int64(1), instrB, "tenant B must have exactly one instrument in its CRM Mongo")

	// Cross-store leak guard (Mongo): A's holder's instrument must not exist in
	// B's Mongo, and vice versa. The collection is namespaced by org id, so we
	// probe both the org-keyed collection and the holder filter to be strict.
	assert.Equal(t, int64(0), instrumentDocsForHolder(t, mongoClient, tenantB.mongoDBName, tenantA.orgID.String(), tenantA.holderID),
		"tenant A's instrument must NOT appear in tenant B's CRM Mongo")
	assert.Equal(t, int64(0), instrumentDocsForHolder(t, mongoClient, tenantA.mongoDBName, tenantB.orgID.String(), tenantB.holderID),
		"tenant B's instrument must NOT appear in tenant A's CRM Mongo")
}

// newCompositionTenant provisions one tenant: a fresh onboarding-PG database
// (migrated, with org + ledger + asset seeded) and a fresh CRM-Mongo database
// (with a holder seeded via the real CRM use case, so the instrument-create
// holder gate passes). It returns the scaffolding the gate asserts against.
func newCompositionTenant(t *testing.T, pgContainer *postgrestestutil.ContainerResult, mongoContainer *mongotestutil.ContainerResult, tenantID, pgDBName, mongoDBName string) *compositionTenant {
	t.Helper()

	// --- onboarding PG: create + migrate the tenant database, seed fixtures ---
	_, err := pgContainer.DB.Exec("CREATE DATABASE " + pgDBName)
	require.NoErrorf(t, err, "failed to create onboarding PG database %s", pgDBName)

	tenantDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgContainer.Host, pgContainer.Port, pgContainer.Config.DBUser, pgContainer.Config.DBPassword, pgDBName)

	tenantPGDB, err := sql.Open("pgx", tenantDSN)
	require.NoError(t, err, "failed to open tenant onboarding PG connection")
	t.Cleanup(func() { _ = tenantPGDB.Close() })
	require.NoError(t, tenantPGDB.Ping(), "failed to ping tenant onboarding PG")

	postgrestestutil.ApplyOnboardingSchema(t, tenantPGDB)

	orgID := postgrestestutil.CreateTestOrganization(t, tenantPGDB)
	ledgerID := postgrestestutil.CreateTestLedger(t, tenantPGDB, orgID)
	postgrestestutil.CreateTestAsset(t, tenantPGDB, orgID, ledgerID, "USD")

	// --- CRM Mongo: seed a holder via the real CRM use case under this tenant's
	// generic MB context (the same key the composition middleware writes). The
	// holder must exist so the instrument-create holder gate (GetHolderByID)
	// passes during the concurrent POST. ---
	mongoConn := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoDBName)
	tenantMongoDB, err := mongoConn.Database(context.Background())
	require.NoError(t, err, "failed to resolve tenant CRM Mongo database")

	cipher := testutils.SetupCrypto(t)
	holderRepo, err := crmholder.NewMongoDBRepository(nil, cipher)
	require.NoError(t, err)
	instrumentRepo, err := crminstrument.NewMongoDBRepository(nil, cipher)
	require.NoError(t, err)
	seedUC := &crmservices.UseCase{HolderRepo: holderRepo, InstrumentRepo: instrumentRepo}

	setupCtx := tmcore.ContextWithMB(context.Background(), tenantMongoDB)

	holder, err := seedUC.CreateHolder(setupCtx, orgID.String(), &mmodel.CreateHolderInput{
		Type:     testutils.Ptr("NATURAL_PERSON"),
		Name:     "Holder " + tenantID,
		Document: "1234567890" + tenantID[len(tenantID)-1:],
	})
	require.NoErrorf(t, err, "failed to seed holder for tenant %s", tenantID)
	require.NotNil(t, holder.ID)

	return &compositionTenant{
		tenantID:    tenantID,
		pgDBName:    pgDBName,
		pgDB:        tenantPGDB,
		orgID:       orgID,
		ledgerID:    ledgerID,
		mongoDBName: mongoDBName,
		holderID:    *holder.ID,
	}
}

// postComposition fires one composition POST for the given tenant, addressing
// the tenant ONLY via the JWT tenantId claim and requesting an instrument.
func postComposition(app *fiber.App, tn *compositionTenant) error {
	body, err := json.Marshal(mmodel.CreateHolderAccountInput{
		Name:           "Composite " + tn.tenantID,
		AssetCode:      "USD",
		Type:           "deposit",
		Alias:          testutils.Ptr("@comp-" + tn.tenantID),
		BankingDetails: &mmodel.BankingDetails{},
	})
	if err != nil {
		return err
	}

	// Org and ledger are path-scoped now; the full target path carries both as
	// validated UUID segments. Tenant is still addressed only via the JWT claim.
	target := "/v1/organizations/" + tn.orgID.String() + "/ledgers/" + tn.ledgerID.String() + "/holders/" + tn.holderID.String() + "/accounts"
	req := httptest.NewRequest(fiber.MethodPost, target, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+compositionTenantJWT(tn.tenantID))

	resp, err := app.Test(req, -1)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusCreated {
		return fmt.Errorf("tenant %s composition POST got %d, want 201: %s", tn.tenantID, resp.StatusCode, string(respBody))
	}

	var out mmodel.HolderAccountResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("tenant %s composition response not JSON: %w", tn.tenantID, err)
	}

	if out.Account == nil {
		return fmt.Errorf("tenant %s composition response missing account", tn.tenantID)
	}

	if out.InstrumentError != nil {
		return fmt.Errorf("tenant %s instrument create failed: %s/%s", tn.tenantID, out.InstrumentError.Status, out.InstrumentError.Reason)
	}

	if out.Instrument == nil {
		return fmt.Errorf("tenant %s composition response missing instrument", tn.tenantID)
	}

	return nil
}

// holderAccountAliases returns the aliases of all non-deleted accounts owned by
// the given holder in the given onboarding PG database, scoped by org + ledger.
func holderAccountAliases(t *testing.T, db *sql.DB, orgID, ledgerID, holderID uuid.UUID) []string {
	t.Helper()

	rows, err := db.Query(
		`SELECT alias FROM account WHERE organization_id = $1 AND ledger_id = $2 AND holder_id = $3 AND deleted_at IS NULL`,
		orgID, ledgerID, holderID)
	require.NoError(t, err, "failed to query accounts by holder")
	defer func() { _ = rows.Close() }()

	return scanAliases(t, rows)
}

// accountAliasesByValue returns all non-deleted account aliases matching the
// given alias value in the given onboarding PG database, regardless of holder.
func accountAliasesByValue(t *testing.T, db *sql.DB, alias string) []string {
	t.Helper()

	rows, err := db.Query(`SELECT alias FROM account WHERE alias = $1 AND deleted_at IS NULL`, alias)
	require.NoError(t, err, "failed to query accounts by alias")
	defer func() { _ = rows.Close() }()

	return scanAliases(t, rows)
}

func scanAliases(t *testing.T, rows *sql.Rows) []string {
	t.Helper()

	var aliases []string

	for rows.Next() {
		var alias sql.NullString

		require.NoError(t, rows.Scan(&alias), "failed to scan account alias")

		if alias.Valid {
			aliases = append(aliases, alias.String)
		}
	}

	require.NoError(t, rows.Err(), "account rows iteration error")

	return aliases
}

// instrumentDocsForHolder counts instrument documents for the given holder in
// the org-keyed instrument collection of the given CRM Mongo database. The
// instrument collection is namespaced "aliases_<orgID>" (legacy collection name
// retained through the F2 Alias->Instrument rename). The stored holder field is
// "holder_id" carrying the UUID value (instrument.MongoDBModel bson tag).
func instrumentDocsForHolder(t *testing.T, client *mongo.Client, mongoDBName, orgID string, holderID uuid.UUID) int64 {
	t.Helper()

	coll := client.Database(mongoDBName).Collection(strings.ToLower("aliases_" + orgID))

	count, err := coll.CountDocuments(context.Background(), bson.D{{Key: "holder_id", Value: holderID}})
	require.NoError(t, err, "failed to count instrument documents")

	return count
}

// newFakeTenantManagerCrossStore returns a tenant-manager stub that, per tenant,
// serves a single TenantConfig carrying BOTH the onboarding PostgreSQL config
// (module key "onboarding") and the CRM MongoDB config (module key "crm-api").
// Resolution within the config is keyed by module, not service, so the same blob
// satisfies both the onboarding PG manager and the CRM Mongo manager. Each config
// points at the shared container with a tenant-specific database name.
func newFakeTenantManagerCrossStore(t *testing.T, pgContainer *postgrestestutil.ContainerResult, mongoContainer *mongotestutil.ContainerResult, tenants map[string]*compositionTenant) *httptest.Server {
	t.Helper()

	pgPort := mustAtoi(t, pgContainer.Port)

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
					PostgreSQL: &tmcore.PostgreSQLConfig{
						Host:     pgContainer.Host,
						Port:     pgPort,
						Database: tn.pgDBName,
						Username: pgContainer.Config.DBUser,
						Password: pgContainer.Config.DBPassword,
						SSLMode:  "disable",
					},
				},
				constant.ModuleCRM: {
					MongoDB: &tmcore.MongoDBConfig{
						URI:      mongoContainer.URI,
						Database: tn.mongoDBName,
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

// compositionTenantJWT builds an unsigned-but-parseable HS256 token carrying the
// tenantId claim the trusted-auth assertion reads via ParseUnverified. The
// signature is never verified, so any key works.
func compositionTenantJWT(tenantID string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenantId": tenantID,
		"sub":      "test-user",
	})

	signed, _ := token.SignedString([]byte("test-signing-key"))

	return signed
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()

	var n int

	_, err := fmt.Sscanf(s, "%d", &n)
	require.NoErrorf(t, err, "failed to parse port %q", s)

	return n
}
