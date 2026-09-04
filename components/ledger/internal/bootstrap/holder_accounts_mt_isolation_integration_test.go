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
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v4/auth/middleware"
	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/sync/errgroup"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// holderAccountsTenant is one tenant's fixture: its own onboarding PostgreSQL
// database and its own onboarding Mongo database, seeded with a holder owning
// accounts in TWO ledgers plus one metadata document.
type holderAccountsTenant struct {
	tenantID    string
	pgDBName    string
	mongoDBName string

	// authToken is signed once at seed time: getHolderAccounts runs inside
	// errgroup goroutines, where a require-based helper must not be called.
	authToken string

	pg *postgrestestutil.ContainerResult

	orgID    uuid.UUID
	ledger1  uuid.UUID
	ledger2  uuid.UUID
	holderID uuid.UUID

	// aliasL1, aliasL2 are the holder's accounts, one per ledger. aliasOther
	// belongs to a different holder in the same organization.
	aliasL1    string
	aliasL2    string
	aliasOther string

	// metadataAccountID is the account whose metadata document is seeded in this
	// tenant's onboarding Mongo, and metadataValue the value only it carries.
	metadataAccountID string
	metadataValue     string
}

// TestIntegration_HolderAccountsConcurrentTenantIsolation drives the org-scoped
// holder-accounts listing through the REAL composition seam — the real
// buildUnifiedRouteSetup, the real holderAccountsReaderAdapter and the real
// route registrar — against two tenants concurrently.
//
// It is the end-to-end counterpart of TestHolderAccountsTenantMiddlewareWiring:
// that test pins which managers the middleware registers, this one proves the
// resulting request actually resolves both per-tenant stores. The final subtest
// pins the negative — the same request on the CRM options does NOT succeed —
// which is the whole reason the holder-accounts role exists.
func TestIntegration_HolderAccountsConcurrentTenantIsolation(t *testing.T) {
	// Both the setup connections and the per-tenant connections the managers open
	// lazily go through the lib-commons clients, which reject plaintext URIs
	// unless ALLOW_INSECURE_TLS=true. The testcontainers speak plaintext.
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	logger := &libLog.GoLogger{}

	tenantAPG := postgrestestutil.SetupMigratedContainer(t, "onboarding")
	tenantBPG := postgrestestutil.SetupMigratedContainer(t, "onboarding")
	mongoContainer := mongotestutil.SetupReusableContainer(t)
	tenantAMongo := mongotestutil.CreateOwnedDatabase(t, mongoContainer)
	tenantBMongo := mongotestutil.CreateOwnedDatabase(t, mongoContainer)

	tenantA := seedHolderAccountsTenant(t, tenantAPG, tenantAMongo, "tenanta")
	tenantB := seedHolderAccountsTenant(t, tenantBPG, tenantBMongo, "tenantb")

	tenants := map[string]*holderAccountsTenant{
		tenantA.tenantID: tenantA,
		tenantB.tenantID: tenantB,
	}

	// Fake tenant-manager control plane: per tenant it serves ONE config carrying
	// the onboarding PostgreSQL AND the onboarding MongoDB under the SAME module
	// key, which is what the holder-accounts middleware resolves.
	tmServer := newFakeTenantManagerOnboardingStores(t, mongoContainer, tenants)
	defer tmServer.Close()

	tenantClient, err := tmclient.NewClient(tmServer.URL, logger,
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tenantClient.Close() })

	onboardingPGManager := tmpostgres.NewManager(tenantClient, constant.ModuleOnboarding,
		tmpostgres.WithModule(constant.ModuleOnboarding), tmpostgres.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingPGManager.Close(context.Background()) })

	onboardingMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleOnboarding,
		tmmongo.WithModule(constant.ModuleOnboarding), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = onboardingMongoManager.Close(context.Background()) })

	// The CRM Mongo manager is built only so the CRM options exist for the
	// negative subtest below; the holder-accounts path never resolves it.
	crmMongoManager := tmmongo.NewManager(tenantClient, constant.ModuleCRM,
		tmmongo.WithModule(constant.ModuleCRM), tmmongo.WithLogger(logger))
	t.Cleanup(func() { _ = crmMongoManager.Close(context.Background()) })

	// The REAL composition root, not a hand-rolled middleware: this is what makes
	// the test fail if buildUnifiedRouteSetup stops binding the onboarding stores.
	setup, err := buildUnifiedRouteSetup(
		&Config{MultiTenantEnabled: true}, logger,
		onboardingPGManager, &tmpostgres.Manager{},
		onboardingMongoManager, &tmmongo.Manager{}, crmMongoManager, &tmmongo.Manager{},
		nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, setup.holderAccountsRouteOptions, "holder-accounts options must be built in multi-tenant mode")

	// MT-style repositories: nil static connection, so every store must come from
	// the request context the middleware populates.
	uc := &query.UseCase{
		AccountRepo:            account.NewAccountPostgreSQLRepository(nil, true),
		OnboardingMetadataRepo: mongodb.NewMetadataMongoDBRepository(nil),
	}
	handler := &httpin.HolderAccountsHandler{Reader: holderAccountsReaderAdapter{query: uc}}

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
		mountCRMHuma(app, middleware.NewAuthClient("", false, nil), nil, nil, handler, nil, nil, routeOptions)

		return app
	}

	app := newApp(setup.holderAccountsRouteOptions)

	t.Run("concurrent_tenants_see_only_their_own_accounts_across_ledgers", func(t *testing.T) {
		var g errgroup.Group

		for _, tn := range []*holderAccountsTenant{tenantA, tenantB} {
			tn := tn
			g.Go(func() error {
				body, status, err := getHolderAccounts(app, tn, "")
				if err != nil {
					return err
				}

				if status != stdhttp.StatusOK {
					return fmt.Errorf("tenant %s: got status %d, body %s", tn.tenantID, status, body)
				}

				aliases, err := aliasesFromListing(body)
				if err != nil {
					return fmt.Errorf("tenant %s: %w", tn.tenantID, err)
				}

				want := []string{tn.aliasL1, tn.aliasL2}
				if !sameSet(aliases, want) {
					return fmt.Errorf("tenant %s: got aliases %v, want %v", tn.tenantID, aliases, want)
				}

				// Stated explicitly rather than implied by the set size: the
				// listing is scoped by holder, not merely by organization.
				for _, alias := range aliases {
					if alias == tn.aliasOther {
						return fmt.Errorf("tenant %s: listing leaked another holder's account %s", tn.tenantID, alias)
					}
				}

				return nil
			})
		}

		require.NoError(t, g.Wait(), "each tenant must see exactly its own holder's accounts, from both of its ledgers")
	})

	t.Run("ledger_id_narrows_within_the_tenant", func(t *testing.T) {
		body, status, err := getHolderAccounts(app, tenantA, tenantA.ledger1.String())
		require.NoError(t, err)
		require.Equal(t, stdhttp.StatusOK, status, "body: %s", body)

		aliases, err := aliasesFromListing(body)
		require.NoError(t, err)
		assert.Equal(t, []string{tenantA.aliasL1}, aliases, "?ledger_id= must narrow the org-wide listing to one ledger")
	})

	t.Run("metadata_comes_from_the_tenants_own_onboarding_mongo", func(t *testing.T) {
		for _, tn := range []*holderAccountsTenant{tenantA, tenantB} {
			body, status, err := getHolderAccounts(app, tn, "")
			require.NoError(t, err)
			require.Equal(t, stdhttp.StatusOK, status, "body: %s", body)

			items, err := itemsFromListing(body)
			require.NoError(t, err)

			var seen int

			for _, item := range items {
				if item["id"] != tn.metadataAccountID {
					continue
				}

				seen++

				meta, ok := item["metadata"].(map[string]any)
				require.Truef(t, ok, "tenant %s: account %s carries no metadata object: %v", tn.tenantID, item["id"], item)
				assert.Equal(t, tn.metadataValue, meta["tenant_marker"],
					"tenant %s must read its metadata from its OWN onboarding Mongo", tn.tenantID)
			}

			assert.Equalf(t, 1, seen, "tenant %s: expected its metadata-bearing account in the listing", tn.tenantID)
		}
	})

	t.Run("ledger_id_from_another_org_narrows_to_empty", func(t *testing.T) {
		// tenantB's ledger is a real UUID that tenantA's organization does not own.
		body, status, err := getHolderAccounts(app, tenantA, tenantB.ledger1.String())
		require.NoError(t, err)
		require.Equal(t, stdhttp.StatusOK, status, "an unowned ledger narrows to nothing, it is not an error; body: %s", body)

		aliases, err := aliasesFromListing(body)
		require.NoError(t, err)
		assert.Empty(t, aliases)
	})

	t.Run("malformed_ledger_id_is_400_with_code_0082", func(t *testing.T) {
		body, status, err := getHolderAccounts(app, tenantA, "not-a-uuid")
		require.NoError(t, err)

		require.Equalf(t, stdhttp.StatusBadRequest, status,
			"a malformed ledger_id is a query-parameter format error, not a missing ledger (404); body: %s", body)

		var problem map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &problem), "body: %s", body)
		assert.Equal(t, constant.ErrInvalidQueryParameter.Error(), problem["code"],
			"malformed ledger_id must carry the invalid-query-parameter code")
	})

	// The regression pin: this route used to run on crmRouteOptions, whose
	// middleware binds no onboarding PostgreSQL at all, so the account read failed
	// with "tenant postgres connection missing from context". Serving 200 here
	// would mean the holder-accounts role has stopped being load-bearing.
	t.Run("same_request_on_crm_options_does_not_succeed", func(t *testing.T) {
		require.NotNil(t, setup.crmRouteOptions)

		crmApp := newApp(setup.crmRouteOptions)

		body, status, err := getHolderAccounts(crmApp, tenantA, "")
		require.NoError(t, err)

		assert.Equalf(t, stdhttp.StatusInternalServerError, status,
			"the CRM options carry no onboarding PostgreSQL, so the account read fails with the "+
				"\"tenant postgres connection missing from context\" 500 the holder-accounts role exists to prevent; body: %s", body)
	})
}

// seedHolderAccountsTenant provisions one tenant's onboarding stores: an
// organization with two ledgers, a holder owning one account in each, a second
// holder's account that must never appear, and one metadata document in the
// tenant's own onboarding Mongo.
func seedHolderAccountsTenant(t *testing.T, pg *postgrestestutil.ContainerResult, mongoDB *mongo.Database, tenantID string) *holderAccountsTenant {
	t.Helper()

	orgID := postgrestestutil.CreateTestOrganization(t, pg.DB)
	ledger1 := postgrestestutil.CreateTestLedger(t, pg.DB, orgID)
	ledger2 := postgrestestutil.CreateTestLedger(t, pg.DB, orgID)

	holderID := uuid.New()
	otherHolderID := uuid.New()

	seed := func(ledgerID uuid.UUID, holder *uuid.UUID, prefix string) (uuid.UUID, string) {
		alias := fmt.Sprintf("@%s-%s-%s", tenantID, prefix, uuid.NewString()[:8])

		params := postgrestestutil.DefaultAccountParams()
		params.Alias = alias
		params.Name = prefix
		params.HolderID = holder

		return postgrestestutil.CreateTestAccountWithParams(t, pg.DB, orgID, ledgerID, params), alias
	}

	idL1, aliasL1 := seed(ledger1, &holderID, "l1")
	_, aliasL2 := seed(ledger2, &holderID, "l2")
	_, aliasOther := seed(ledger1, &otherHolderID, "other")

	// One metadata document, in this tenant's own onboarding Mongo, carrying a
	// value unique to the tenant so a cross-tenant read is visible.
	metadataValue := "marker-" + tenantID
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	_, err := mongoDB.Collection(strings.ToLower(constant.EntityAccount)).InsertOne(context.Background(), map[string]any{
		"entity_id":   idL1.String(),
		"entity_name": constant.EntityAccount,
		"metadata":    map[string]any{"tenant_marker": metadataValue},
		"created_at":  now,
		"updated_at":  now,
	})
	require.NoError(t, err, "failed to seed onboarding metadata for %s", tenantID)

	return &holderAccountsTenant{
		tenantID:          tenantID,
		authToken:         tenantJWT(t, tenantID),
		pgDBName:          pg.Config.DBName,
		mongoDBName:       mongoDB.Name(),
		pg:                pg,
		orgID:             orgID,
		ledger1:           ledger1,
		ledger2:           ledger2,
		holderID:          holderID,
		aliasL1:           aliasL1,
		aliasL2:           aliasL2,
		aliasOther:        aliasOther,
		metadataAccountID: idL1.String(),
		metadataValue:     metadataValue,
	}
}

// getHolderAccounts issues the org-scoped listing for a tenant, authenticated by
// the JWT tenantId claim the trusted-auth assertion reads.
func getHolderAccounts(app *fiber.App, tn *holderAccountsTenant, ledgerID string) (string, int, error) {
	url := fmt.Sprintf("/v2/organizations/%s/holders/%s/accounts", tn.orgID, tn.holderID)
	if ledgerID != "" {
		url += "?ledger_id=" + ledgerID
	}

	req := httptest.NewRequest(stdhttp.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+tn.authToken)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
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

// itemsFromListing decodes the paginated listing envelope into its items.
func itemsFromListing(body string) ([]map[string]any, error) {
	var envelope struct {
		Items []map[string]any `json:"items"`
	}

	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, fmt.Errorf("decode listing: %w (body %s)", err, body)
	}

	return envelope.Items, nil
}

func aliasesFromListing(body string) ([]string, error) {
	items, err := itemsFromListing(body)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(items))

	for _, item := range items {
		alias, _ := item["alias"].(string)
		out = append(out, alias)
	}

	return out, nil
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	seen := make(map[string]int, len(got))
	for _, g := range got {
		seen[g]++
	}

	for _, w := range want {
		if seen[w] == 0 {
			return false
		}

		seen[w]--
	}

	return true
}

// newFakeTenantManagerOnboardingStores serves, per tenant, one TenantConfig whose
// onboarding module carries BOTH the PostgreSQL and the MongoDB coordinates — the
// exact shape the holder-accounts middleware resolves. The CRM module carries the
// Mongo only, matching production, so the negative subtest exercises a realistic
// CRM config rather than an empty one.
func newFakeTenantManagerOnboardingStores(t *testing.T, mongoContainer *mongotestutil.ContainerResult, tenants map[string]*holderAccountsTenant) *httptest.Server {
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
					PostgreSQL: &tmcore.PostgreSQLConfig{
						Host:     tn.pg.Host,
						Port:     mustAtoiPort(t, tn.pg.Port),
						Database: tn.pgDBName,
						Username: tn.pg.Config.DBUser,
						Password: tn.pg.Config.DBPassword,
						SSLMode:  "disable",
					},
					MongoDB: &tmcore.MongoDBConfig{
						URI:      mongoContainer.URI,
						Database: tn.mongoDBName,
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

func mustAtoiPort(t *testing.T, s string) int {
	t.Helper()

	var n int

	_, err := fmt.Sscanf(s, "%d", &n)
	require.NoErrorf(t, err, "failed to parse port %q", s)

	return n
}
