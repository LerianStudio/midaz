//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/sync/errgroup"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// compositionTenant is one tenant's fixture for the holder-account composition
// POST. Every store the path touches gets its OWN database, so an assertion can
// tell which one a write actually landed in: the onboarding PostgreSQL (account),
// the transaction PostgreSQL (default balance), the onboarding Mongo (account
// metadata) and the CRM Mongo (instrument). Same-database fixtures would hide the
// two defects this test exists to catch.
type compositionTenant struct {
	tenantID string

	// authToken is signed once at seed time: the POST runs inside errgroup
	// goroutines, where a require-based helper must not be called.
	authToken string

	onboardingPG  *postgrestestutil.ContainerResult
	transactionPG *postgrestestutil.ContainerResult

	onboardingMongo *mongo.Database
	crmMongo        *mongo.Database

	orgID    uuid.UUID
	ledgerID uuid.UUID
	holderID uuid.UUID

	// metadataValue is unique per tenant, so a document read from the wrong
	// tenant's store is visible rather than merely plausible.
	metadataValue string
}

// TestIntegration_CompositionMultiTenantStores drives the holder-account POST
// through the REAL composition root — the real buildUnifiedRouteSetup, the real
// route registrar and the real CreateAccount use case — against two tenants whose
// four stores are four distinct databases.
//
// It is the end-to-end counterpart of TestCompositionTenantMiddlewareWiring, and it
// pins the two defects that shipped together on this route in multi-tenant mode:
//
//   - the composition middleware carried no transaction PostgreSQL, so the default
//     balance CreateAccount always writes failed requireTenant and the whole POST
//     returned 500/0127;
//   - the middleware carried no MODULE-keyed onboarding Mongo, so the account
//     metadata write fell through to the generic key — the CRM Mongo — and would
//     have landed in the wrong store silently once the balance stopped failing
//     first.
func TestIntegration_CompositionMultiTenantStores(t *testing.T) {
	// Both the setup connections and the per-tenant connections the managers open
	// lazily go through the lib-commons clients, which reject plaintext URIs
	// unless ALLOW_INSECURE_TLS=true. The testcontainers speak plaintext.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	logger := &libLog.GoLogger{}

	mongoContainer := mongotestutil.SetupReusableContainer(t)

	tenantA := seedCompositionTenant(t, mongoContainer, "tenanta")
	tenantB := seedCompositionTenant(t, mongoContainer, "tenantb")

	tenants := map[string]*compositionTenant{
		tenantA.tenantID: tenantA,
		tenantB.tenantID: tenantB,
	}

	// Fake tenant-manager control plane: per tenant it serves one config whose
	// onboarding, transaction and crm modules each point at that tenant's own
	// database — the shape the composition middleware resolves.
	tmServer := newFakeTenantManagerCompositionStores(t, mongoContainer, tenants)
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

	crmMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleCRM,
		tmmongo.WithModule(constant.ModuleCRM), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = crmMongoManager.Close(context.Background()) })

	// The REAL composition root, not a hand-rolled middleware: this is what makes
	// the test fail if buildUnifiedRouteSetup stops binding one of the four stores.
	setup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		onboardingPGManager, transactionPGManager,
		onboardingMongoManager, &tmmongo.Manager{}, crmMongoManager, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, setup.compositionRouteOptions, "composition options must be built in multi-tenant mode")

	// MT-style repositories: nil static connection with requireTenant set, so every
	// store must come from the request context the middleware populates.
	uc := &command.UseCase{
		LedgerRepo:             ledger.NewLedgerPostgreSQLRepository(nil, true),
		AssetRepo:              asset.NewAssetPostgreSQLRepository(nil, true),
		AccountRepo:            account.NewAccountPostgreSQLRepository(nil, true),
		BalanceRepo:            balance.NewBalancePostgreSQLRepository(nil, false, true),
		OnboardingMetadataRepo: onbMongo.NewMetadataMongoDBRepository(nil),
	}

	handler := &httpin.CompositionHandler{
		Service: composition.NewService(uc, compositionInstrumentProbe{}),
	}

	newApp := func(routeOptions *http.ProtectedRouteOptions) *fiber.App {
		app := fiber.New(fiber.Config{
			ErrorHandler: func(ctx fiber.Ctx, err error) error {
				return libHTTP.FiberErrorHandler(ctx, err)
			},
		})
		app.Use(http.WithRecover(http.WithRecoverLogger(logger)))
		t.Cleanup(func() { _ = app.Shutdown() })

		// Auth disabled: Authorize is a pass-through, so the post-auth chain the
		// options carry is what the request actually runs.
		mountCompositionHuma(app, middleware.NewAuthClient("", false, nil), handler, routeOptions)

		return app
	}

	app := newApp(setup.compositionRouteOptions)

	t.Run("concurrent_tenants_each_write_all_four_of_their_own_stores", func(t *testing.T) {
		var g errgroup.Group

		// created is written from the errgroup workers and read after Wait, so the
		// writes are mutex-guarded: two goroutines assigning into the same map is a
		// data race regardless of the keys being distinct.
		var createdMu sync.Mutex

		created := make(map[string]string, len(tenants))

		for _, tn := range []*compositionTenant{tenantA, tenantB} {
			tn := tn
			g.Go(func() error {
				body, status, err := postHolderAccount(app, tn, tn.alias("ok"))
				if err != nil {
					return err
				}

				if status != stdhttp.StatusCreated {
					return fmt.Errorf("tenant %s: got status %d, body %s", tn.tenantID, status, body)
				}

				accountID, err := accountIDFromComposition(body)
				if err != nil {
					return fmt.Errorf("tenant %s: %w", tn.tenantID, err)
				}

				createdMu.Lock()
				created[tn.tenantID] = accountID
				createdMu.Unlock()

				return nil
			})
		}

		require.NoError(t, g.Wait(), "the composition POST must succeed for both tenants concurrently")

		for _, tn := range []*compositionTenant{tenantA, tenantB} {
			accountID := created[tn.tenantID]
			require.NotEmpty(t, accountID, "tenant %s: response carried no account id", tn.tenantID)

			// 1. The account, in this tenant's onboarding PostgreSQL, owned by the path holder.
			var holderID string
			require.NoError(t,
				tn.onboardingPG.DB.QueryRow(`SELECT holder_id FROM account WHERE id = $1`, accountID).Scan(&holderID),
				"tenant %s: the account must be persisted in its own onboarding PostgreSQL", tn.tenantID)
			assert.Equal(t, tn.holderID.String(), holderID,
				"tenant %s: ownership is path-sourced, so the account must carry the path holder", tn.tenantID)

			// 2. The default balance, in this tenant's TRANSACTION PostgreSQL. This is the
			// regression pin: the composition middleware used to carry no transaction PG,
			// so this write failed requireTenant and the POST returned 500/0127.
			var balances int
			require.NoError(t,
				tn.transactionPG.DB.QueryRow(
					`SELECT count(*) FROM balance WHERE account_id = $1 AND key = $2`,
					accountID, constant.DefaultBalanceKey,
				).Scan(&balances),
				"tenant %s: failed to read its transaction PostgreSQL", tn.tenantID)
			assert.Equal(t, 1, balances,
				"tenant %s: CreateAccount always writes the default balance, and it must land in the tenant's "+
					"transaction PostgreSQL", tn.tenantID)

			// 3. The account metadata, in this tenant's ONBOARDING Mongo, read back by the
			// tenant-unique marker.
			metaDoc := tn.onboardingMongo.Collection(strings.ToLower(constant.EntityAccount)).
				FindOne(context.Background(), bson.M{"entity_id": accountID})

			var meta struct {
				Metadata map[string]any `bson:"metadata"`
			}
			require.NoError(t, metaDoc.Decode(&meta),
				"tenant %s: the account metadata must be written to its own onboarding Mongo", tn.tenantID)
			assert.Equal(t, tn.metadataValue, meta.Metadata["tenant_marker"],
				"tenant %s: metadata must round-trip from its own onboarding Mongo", tn.tenantID)

			// 4. The anti-cross-store assertion, and the reason the module-keyed onboarding
			// Mongo is not optional: the metadata repo resolves the module key FIRST and
			// falls back to the generic key. With only the CRM Mongo on the generic key, this
			// document would have landed in the CRM store — no error, wrong store.
			crmMetadata, err := tn.crmMongo.Collection(strings.ToLower(constant.EntityAccount)).
				CountDocuments(context.Background(), bson.M{"entity_id": accountID})
			require.NoError(t, err, "tenant %s: failed to read its CRM Mongo", tn.tenantID)
			assert.Zero(t, crmMetadata,
				"tenant %s: account metadata must never reach the CRM Mongo — the module-keyed onboarding Mongo is "+
					"what keeps the generic-key fallback from writing to the wrong store", tn.tenantID)

			// 5. The instrument, on the GENERIC Mongo key, which must still resolve to the
			// CRM store now that a module-keyed Mongo shares the middleware.
			instruments, err := tn.crmMongo.Collection(compositionInstrumentCollection).
				CountDocuments(context.Background(), bson.M{"account_id": accountID})
			require.NoError(t, err, "tenant %s: failed to read its CRM Mongo", tn.tenantID)
			assert.Equal(t, int64(1), instruments,
				"tenant %s: the instrument write reads the generic Mongo key, which must still resolve to the CRM "+
					"store alongside the module-keyed onboarding Mongo", tn.tenantID)
		}
	})

	t.Run("no_store_carries_the_other_tenants_account", func(t *testing.T) {
		body, status, err := postHolderAccount(app, tenantA, tenantA.alias("iso"))
		require.NoError(t, err)
		require.Equal(t, stdhttp.StatusCreated, status, "body: %s", body)

		accountID, err := accountIDFromComposition(body)
		require.NoError(t, err)

		var accounts int
		require.NoError(t, tenantB.onboardingPG.DB.QueryRow(
			`SELECT count(*) FROM account WHERE id = $1`, accountID,
		).Scan(&accounts))
		assert.Zero(t, accounts, "tenant B's onboarding PostgreSQL must not carry tenant A's account")

		var balances int
		require.NoError(t, tenantB.transactionPG.DB.QueryRow(
			`SELECT count(*) FROM balance WHERE account_id = $1`, accountID,
		).Scan(&balances))
		assert.Zero(t, balances, "tenant B's transaction PostgreSQL must not carry tenant A's balance")

		metadata, err := tenantB.onboardingMongo.Collection(strings.ToLower(constant.EntityAccount)).
			CountDocuments(context.Background(), bson.M{"entity_id": accountID})
		require.NoError(t, err)
		assert.Zero(t, metadata, "tenant B's onboarding Mongo must not carry tenant A's account metadata")

		instruments, err := tenantB.crmMongo.Collection(compositionInstrumentCollection).
			CountDocuments(context.Background(), bson.M{"account_id": accountID})
		require.NoError(t, err)
		assert.Zero(t, instruments, "tenant B's CRM Mongo must not carry tenant A's instrument")
	})

	// The compensation regression: the holder-accounts options carry the onboarding
	// stores but NO transaction PostgreSQL, which is exactly the context the
	// composition route ran in before the fix. The balance write fails, the POST is
	// 500/0127, and the compensating delete must leave no account behind.
	t.Run("without_the_transaction_pg_the_post_is_500_0127_and_the_account_is_compensated", func(t *testing.T) {
		require.NotNil(t, setup.holderAccountsRouteOptions)

		alias := tenantA.alias("nobalance")

		body, status, err := postHolderAccount(newApp(setup.holderAccountsRouteOptions), tenantA, alias)
		require.NoError(t, err)

		require.Equalf(t, stdhttp.StatusInternalServerError, status,
			"a context without the transaction PostgreSQL fails the default balance, which is the "+
				"\"tenant postgres connection missing from context\" 500 this route's own middleware exists to "+
				"prevent; body: %s", body)

		var problem map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &problem), "body: %s", body)
		assert.Equal(t, constant.ErrAccountCreationFailed.Error(), problem["code"],
			"a failed default balance must surface as the account-creation-failed code")

		var accounts int
		require.NoError(t, tenantA.onboardingPG.DB.QueryRow(
			`SELECT count(*) FROM account WHERE alias = $1 AND deleted_at IS NULL`, alias,
		).Scan(&accounts))
		assert.Zero(t, accounts,
			"the balance failure must compensate the account: a persisted account with no default balance is "+
				"unusable and invisible to the client that got the 500")
	})
}

// compositionInstrumentCollection is where the instrument probe writes. The name
// only has to be stable between the probe and its assertions.
const compositionInstrumentCollection = "instrument"

// compositionInstrumentProbe stands in for the CRM instrument use case at the one
// axis this test is about: which store the write resolves to. It reads the GENERIC
// Mongo key exactly as the real CRM instrument repository does, so a middleware
// that stopped binding the CRM Mongo on the generic key fails here. Instrument
// validation, encryption and the CRM domain rules are covered by
// TestIntegration_CRMCollapse; reproducing them here would test the CRM, not the
// store wiring.
type compositionInstrumentProbe struct{}

func (compositionInstrumentProbe) CreateInstrument(ctx context.Context, organizationID string, holderID uuid.UUID, in *mmodel.CreateInstrumentInput) (*mmodel.Instrument, error) {
	db := tmcore.GetMBContext(ctx)
	if db == nil {
		return nil, fmt.Errorf("tenant mongo database missing from context on the generic key")
	}

	id := uuid.New()

	_, err := db.Collection(compositionInstrumentCollection).InsertOne(ctx, bson.M{
		"id":              id.String(),
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
		"account_id":      in.AccountID,
	})
	if err != nil {
		return nil, err
	}

	return &mmodel.Instrument{ID: &id, LedgerID: &in.LedgerID, AccountID: &in.AccountID}, nil
}

// alias builds a per-tenant, per-case account alias so two cases in the same
// tenant's database cannot collide on the alias uniqueness check.
func (tn *compositionTenant) alias(purpose string) string {
	return fmt.Sprintf("@%s-%s-%s", tn.tenantID, purpose, uuid.NewString()[:8])
}

// seedCompositionTenant provisions one tenant's four databases: the onboarding
// PostgreSQL (organization, ledger and the USD asset the create path validates),
// the transaction PostgreSQL (migrated, empty — the balance is what the POST
// writes), and one Mongo database each for onboarding metadata and CRM.
func seedCompositionTenant(t *testing.T, mongoContainer *mongotestutil.ContainerResult, tenantID string) *compositionTenant {
	t.Helper()

	onboardingPG := postgrestestutil.SetupMigratedContainer(t, "onboarding")
	transactionPG := postgrestestutil.SetupMigratedContainer(t, "transaction")

	orgID := postgrestestutil.CreateTestOrganization(t, onboardingPG.DB)
	ledgerID := postgrestestutil.CreateTestLedger(t, onboardingPG.DB, orgID)
	postgrestestutil.CreateTestAsset(t, onboardingPG.DB, orgID, ledgerID, "USD")

	return &compositionTenant{
		tenantID:        tenantID,
		authToken:       tenantJWT(t, tenantID),
		onboardingPG:    onboardingPG,
		transactionPG:   transactionPG,
		onboardingMongo: mongotestutil.CreateOwnedDatabase(t, mongoContainer),
		crmMongo:        mongotestutil.CreateOwnedDatabase(t, mongoContainer),
		orgID:           orgID,
		ledgerID:        ledgerID,
		holderID:        uuid.New(),
		metadataValue:   "marker-" + tenantID,
	}
}

// mountCompositionHuma mounts the REAL composition registrar on a bare Fiber app,
// mirroring how NewUnifiedServer hosts the route: the Fiber guard chain from
// routeOptions runs before the Huma terminal.
func mountCompositionHuma(app *fiber.App, auth *middleware.AuthClient, ch *httpin.CompositionHandler, routeOptions *http.ProtectedRouteOptions) {
	libProblem.Install()
	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "composition-integration", Version: "test", Servers: []string{"/v2"}})
	http.InstallLedgerSchemaNamer(hAPI)

	httpin.RegisterCompositionV2RoutesToApp(apiV2, hAPI, auth, ch, routeOptions)
}

// postHolderAccount issues the composition POST for a tenant, authenticated by the
// JWT tenantId claim the trusted-auth assertion reads. The body carries metadata
// (so the onboarding metadata write actually happens — a nil map short-circuits it)
// and banking details (so the instrument write happens).
func postHolderAccount(app *fiber.App, tn *compositionTenant, alias string) (string, int, error) {
	payload, err := json.Marshal(mmodel.CreateHolderAccountInput{
		Name:      "composition account",
		AssetCode: "USD",
		Type:      "deposit",
		Alias:     &alias,
		Metadata:  map[string]any{"tenant_marker": tn.metadataValue},
		BankingDetails: &mmodel.BankingDetails{
			Branch:  testutils.Ptr("0001"),
			Account: testutils.Ptr("123450"),
		},
	})
	if err != nil {
		return "", 0, err
	}

	url := fmt.Sprintf("/v2/organizations/%s/ledgers/%s/holders/%s/accounts", tn.orgID, tn.ledgerID, tn.holderID)

	req := httptest.NewRequest(stdhttp.MethodPost, url, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tn.authToken)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 60 * time.Second})
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	return string(body), resp.StatusCode, nil
}

// accountIDFromComposition reads the created account's ID out of the composite
// 201 body.
func accountIDFromComposition(body string) (string, error) {
	var envelope struct {
		Account *struct {
			ID string `json:"id"`
		} `json:"account"`
		InstrumentError *struct {
			Reason string `json:"reason"`
		} `json:"instrumentError"`
	}

	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return "", fmt.Errorf("decode composition response: %w (body %s)", err, body)
	}

	if envelope.InstrumentError != nil {
		return "", fmt.Errorf("instrument write failed with reason %q (body %s)", envelope.InstrumentError.Reason, body)
	}

	if envelope.Account == nil || envelope.Account.ID == "" {
		return "", fmt.Errorf("composition response carries no account (body %s)", body)
	}

	return envelope.Account.ID, nil
}

// newFakeTenantManagerCompositionStores serves, per tenant, one TenantConfig whose
// onboarding module carries the onboarding PostgreSQL AND the onboarding MongoDB,
// whose transaction module carries the transaction PostgreSQL, and whose crm module
// carries the CRM MongoDB — four distinct databases, so an assertion can tell which
// store a write reached.
func newFakeTenantManagerCompositionStores(t *testing.T, mongoContainer *mongotestutil.ContainerResult, tenants map[string]*compositionTenant) *httptest.Server {
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
					MongoDB: &tmcore.MongoDBConfig{
						URI:      mongoContainer.URI,
						Database: tn.onboardingMongo.Name(),
					},
				},
				constant.ModuleTransaction: {
					PostgreSQL: compositionPGConfig(t, tn.transactionPG),
				},
				constant.ModuleCRM: {
					MongoDB: &tmcore.MongoDBConfig{
						URI:      mongoContainer.URI,
						Database: tn.crmMongo.Name(),
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

// compositionPGConfig projects a test container onto the tenant-manager PostgreSQL
// coordinates.
func compositionPGConfig(t *testing.T, pg *postgrestestutil.ContainerResult) *tmcore.PostgreSQLConfig {
	t.Helper()

	return &tmcore.PostgreSQLConfig{
		Host:     pg.Host,
		Port:     mustAtoiPort(t, pg.Port),
		Database: pg.Config.DBName,
		Username: pg.Config.DBUser,
		Password: pg.Config.DBPassword,
		SSLMode:  "disable",
	}
}
