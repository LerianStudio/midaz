// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/middleware"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tenantpool"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/LerianStudio/midaz/v4/tests/utils/stubs"
)

// These tests drive the multi-tenant account surface through the production post-auth
// chain [MarkTrustedAuthAssertion, WithTenantDB, WithTenantPoolResolution] over a real
// tmpostgres.Manager, a fake tenant-manager control plane and per-tenant PostgreSQL
// databases. The tenant manager is made to close a tenant's pool in the middle of a
// request by a decorator around the account repository that runs a hook right before
// the account INSERT — after the ledger settings, asset and alias reads have already
// run on the pool WithTenantDB pinned.
//
// MUST-NOT-PARALLELIZE: libProblem.Install() swaps the process-global huma.NewError.

// tenantPoolSwapLogMessage is a copy of tenantpool.poolSwapLogMessage (unexported, in
// components/ledger/internal/adapters/postgres/tenantpool/tenantpool.go); keep the two
// in sync. The tests count it to prove the swap was observed exactly when expected.
const tenantPoolSwapLogMessage = "Tenant PostgreSQL pool changed during request; using the current pool"

// tenantPoolRequestTimeout bounds every waiting point so a broken scenario fails
// instead of hanging the suite.
const tenantPoolRequestTimeout = 60 * time.Second

var errTenantPoolManagerUnavailable = errors.New("tenant manager unavailable")

// tenantPoolFakeMode is how the fake tenant-manager answers for one tenant.
type tenantPoolFakeMode int

const (
	tenantPoolFakeActive tenantPoolFakeMode = iota
	tenantPoolFakeNotFound
	tenantPoolFakeUnavailable
)

// tenantPoolFakeTenant is the fake's mutable view of one tenant: which database its
// onboarding PostgreSQL config points at and how the fake answers for it.
type tenantPoolFakeTenant struct {
	dbName string
	mode   tenantPoolFakeMode
}

// tenantPoolFake is a tenant-manager control plane whose answers can change between
// requests: it can repoint a tenant at another database, report it as not found
// (suspended/removed) or fail as unavailable.
type tenantPoolFake struct {
	server *httptest.Server

	host     string
	port     int
	user     string
	password string

	mu      sync.Mutex
	tenants map[string]*tenantPoolFakeTenant
}

func newTenantPoolFake(t *testing.T, pg *postgrestestutil.ContainerResult) *tenantPoolFake {
	t.Helper()

	f := &tenantPoolFake{
		host:     pg.Host,
		port:     mustAtoi(t, pg.Port),
		user:     pg.Config.DBUser,
		password: pg.Config.DBPassword,
		tenants:  map[string]*tenantPoolFakeTenant{},
	}

	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/v1/tenants/", f.serve)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	return f
}

func (f *tenantPoolFake) addTenant(tenantID, dbName string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.tenants[tenantID] = &tenantPoolFakeTenant{dbName: dbName, mode: tenantPoolFakeActive}
}

func (f *tenantPoolFake) pointTo(tenantID, dbName string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.tenants[tenantID].dbName = dbName
}

func (f *tenantPoolFake) setMode(tenantID string, mode tenantPoolFakeMode) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.tenants[tenantID].mode = mode
}

func (f *tenantPoolFake) serve(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	// Path: /v1/tenants/{tenantID}/associations/{service}/connections
	parts := strings.FieldsFunc(r.URL.Path, func(c rune) bool { return c == '/' })
	if len(parts) < 5 || parts[0] != "v1" || parts[1] != "tenants" || parts[3] != "associations" {
		stdhttp.Error(w, "invalid path", stdhttp.StatusBadRequest)

		return
	}

	f.mu.Lock()
	tn, ok := f.tenants[parts[2]]

	var snapshot tenantPoolFakeTenant
	if ok {
		snapshot = *tn
	}
	f.mu.Unlock()

	if !ok || snapshot.mode == tenantPoolFakeNotFound {
		stdhttp.Error(w, "tenant not found", stdhttp.StatusNotFound)

		return
	}

	if snapshot.mode == tenantPoolFakeUnavailable {
		stdhttp.Error(w, "tenant manager unavailable", stdhttp.StatusServiceUnavailable)

		return
	}

	config := &tmcore.TenantConfig{
		ID:         parts[2],
		TenantSlug: parts[2],
		Status:     "active",
		Databases: map[string]tmcore.DatabaseConfig{
			constant.ModuleOnboarding: {
				PostgreSQL: &tmcore.PostgreSQLConfig{
					Host:     f.host,
					Port:     f.port,
					Database: snapshot.dbName,
					Username: f.user,
					Password: f.password,
					SSLMode:  "disable",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(config); err != nil {
		stdhttp.Error(w, "encode failed", stdhttp.StatusInternalServerError)
	}
}

// newTenantPoolClient returns a tenant-manager client over the fake.
func newTenantPoolClient(t *testing.T, fake *tenantPoolFake) *tmclient.Client {
	t.Helper()

	client, err := tmclient.NewClient(fake.server.URL, &libLog.GoLogger{},
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// newTenantPoolManager returns the production-shaped onboarding PG manager.
func newTenantPoolManager(t *testing.T, client *tmclient.Client) *tmpostgres.Manager {
	t.Helper()

	manager := tmpostgres.NewManager(client, constant.ModuleOnboarding,
		tmpostgres.WithModule(constant.ModuleOnboarding), tmpostgres.WithLogger(&libLog.GoLogger{}))
	t.Cleanup(func() { _ = manager.Close(context.Background()) })

	return manager
}

// tenantPoolTenant is one tenant's onboarding database with an org, ledger and
// USD asset seeded, ready for account creation.
type tenantPoolTenant struct {
	id       string
	pg       *postgrestestutil.ContainerResult
	orgID    uuid.UUID
	ledgerID uuid.UUID
}

func newTenantPoolTenant(t *testing.T, tenantID string) *tenantPoolTenant {
	t.Helper()

	pg := postgrestestutil.SetupMigratedContainer(t, "onboarding")

	orgID := postgrestestutil.CreateTestOrganization(t, pg.DB)
	ledgerID := postgrestestutil.CreateTestLedger(t, pg.DB, orgID)
	postgrestestutil.CreateTestAsset(t, pg.DB, orgID, ledgerID, "USD")

	return &tenantPoolTenant{id: tenantID, pg: pg, orgID: orgID, ledgerID: ledgerID}
}

// tenantPoolLogEntry is one captured log line.
type tenantPoolLogEntry struct {
	level  int
	msg    string
	fields map[string]any
}

// tenantPoolLogRecorder captures every log line emitted under the request
// context. It satisfies libLog.Universal, which is all ContextWithLogger needs.
type tenantPoolLogRecorder struct {
	mu      sync.Mutex
	entries []tenantPoolLogEntry
}

func (r *tenantPoolLogRecorder) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := tenantPoolLogEntry{level: level, msg: msg, fields: make(map[string]any, len(fields))}

	for _, f := range fields {
		if field, ok := f.(libLog.Field); ok {
			entry.fields[field.Key] = field.Value
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, entry)
}

// swapWarnings counts the pool-swap warnings logged for tenantID.
func (r *tenantPoolLogRecorder) swapWarnings(tenantID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0

	for _, e := range r.entries {
		if e.msg == tenantPoolSwapLogMessage && e.level == libLog.LevelWarn && e.fields[obsconst.AttrKeyTenantID] == tenantID {
			count++
		}
	}

	return count
}

// allSwapWarnings counts every pool-swap warning, whatever the tenant.
func (r *tenantPoolLogRecorder) allSwapWarnings() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0

	for _, e := range r.entries {
		if e.msg == tenantPoolSwapLogMessage {
			count++
		}
	}

	return count
}

// tenantPoolSwapAccountRepo is the test-only swap hook: it runs beforeCreate right
// before delegating the account INSERT to the real repository.
type tenantPoolSwapAccountRepo struct {
	account.Repository

	mu           sync.Mutex
	beforeCreate func(ctx context.Context)
}

func (r *tenantPoolSwapAccountRepo) setBeforeCreate(fn func(ctx context.Context)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.beforeCreate = fn
}

func (r *tenantPoolSwapAccountRepo) Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error) {
	r.mu.Lock()
	hook := r.beforeCreate
	r.mu.Unlock()

	if hook != nil {
		hook(ctx)
	}

	return r.Repository.Create(ctx, acc)
}

// tenantPoolMetadataStub stands in for the onboarding Mongo metadata store, which
// this PG-only chain does not resolve: creates carry no metadata and reads find none.
type tenantPoolMetadataStub struct {
	mongodb.Repository
}

func (tenantPoolMetadataStub) Create(context.Context, string, *mongodb.Metadata) error { return nil }

func (tenantPoolMetadataStub) FindByEntity(context.Context, string, string) (*mongodb.Metadata, error) {
	return nil, nil
}

// tenantPoolGatedResolver wraps the manager as the adapter's Resolver and can be
// switched to fail every resolution, standing in for a tenant manager that cannot
// be reached while the pinned pool is still cached and open.
type tenantPoolGatedResolver struct {
	inner       tenantpool.Resolver
	unavailable atomic.Bool
	failures    atomic.Int32
}

func (r *tenantPoolGatedResolver) GetDB(ctx context.Context, tenantID string) (dbresolver.DB, error) {
	if r.unavailable.Load() {
		r.failures.Add(1)

		return nil, errTenantPoolManagerUnavailable
	}

	return r.inner.GetDB(ctx, tenantID)
}

// tenantPoolHarnessConfig selects the chain under test.
type tenantPoolHarnessConfig struct {
	// withPoolResolution mounts WithTenantPoolResolution as the third post-auth
	// handler; false reproduces the chain as it was before this change.
	withPoolResolution bool
	// resolver, when set, is the Resolver the adapter uses instead of the manager.
	resolver tenantpool.Resolver
}

// tenantPoolHarness is the account surface on /v1 and /v2 behind the multi-tenant chain.
type tenantPoolHarness struct {
	app         *fiber.App
	manager     *tmpostgres.Manager
	fake        *tenantPoolFake
	logs        *tenantPoolLogRecorder
	accountRepo *tenantPoolSwapAccountRepo
}

func newTenantPoolHarness(t *testing.T, fake *tenantPoolFake, cfg tenantPoolHarnessConfig) *tenantPoolHarness {
	t.Helper()

	client := newTenantPoolClient(t, fake)
	manager := newTenantPoolManager(t, client)

	tenantCache := tenantcache.NewTenantCache()
	tenantLoader := tenantcache.NewTenantLoader(client, tenantCache, constant.ModuleOnboarding, time.Minute, &libLog.GoLogger{})

	tenantMiddleware := tmmiddleware.NewTenantMiddleware(
		tmmiddleware.WithPG(manager, constant.ModuleOnboarding),
		tmmiddleware.WithTenantCache(tenantCache),
		tmmiddleware.WithTenantLoader(tenantLoader),
	)

	postAuth := []fiber.Handler{midazhttp.MarkTrustedAuthAssertion(), tenantMiddleware.WithTenantDB}

	if cfg.withPoolResolution {
		var resolver tenantpool.Resolver = manager
		if cfg.resolver != nil {
			resolver = cfg.resolver
		}

		// Same closure as the bootstrap's tenantPoolModule.
		postAuth = append(postAuth, midazhttp.WithTenantPoolResolution(midazhttp.TenantPoolModule{
			Name: constant.ModuleOnboarding,
			New: func(_ context.Context, tenantID string, pinned dbresolver.DB) dbresolver.DB {
				return tenantpool.New(resolver, tenantID, constant.ModuleOnboarding, pinned)
			},
		}))
	}

	routeOptions := &midazhttp.ProtectedRouteOptions{PostAuthMiddlewares: postAuth}

	accountRepo := &tenantPoolSwapAccountRepo{Repository: account.NewAccountPostgreSQLRepository(nil, true)}
	metadataRepo := tenantPoolMetadataStub{}

	commandUC := &command.UseCase{
		OrganizationRepo:       organization.NewOrganizationPostgreSQLRepository(nil, true),
		LedgerRepo:             ledger.NewLedgerPostgreSQLRepository(nil, true),
		AssetRepo:              asset.NewAssetPostgreSQLRepository(nil, true),
		AccountRepo:            accountRepo,
		PortfolioRepo:          portfolio.NewPortfolioPostgreSQLRepository(nil, true),
		SegmentRepo:            segment.NewSegmentPostgreSQLRepository(nil, true),
		OnboardingMetadataRepo: metadataRepo,
		BalanceRepo:            stubs.NewBalanceRepoStub(),
	}
	queryUC := &query.UseCase{
		AccountRepo:            accountRepo,
		OnboardingMetadataRepo: metadataRepo,
	}
	handler := &AccountHandler{Command: commandUC, Query: queryUC}

	logs := &tenantPoolLogRecorder{}

	app := fiber.New()
	t.Cleanup(func() { _ = app.Shutdown() })

	libProblem.Install()
	midazhttp.InstallHumaFrameworkErrors()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	// Every log line of the request, the adapter's swap warning included, lands here.
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(libObservability.ContextWithLogger(c.Context(), logs))

		return c.Next()
	})

	auth := middleware.NewAuthClient("", false, nil)

	apiV1 := app.Group("/v1")
	v1API := openapi.New(app, apiV1, openapi.Config{Title: "tenant-pool-v1", Version: "test", Servers: []string{"/v1"}})
	midazhttp.InstallLedgerSchemaNamer(v1API)
	RegisterAccountRoutesToApp(apiV1, v1API, auth, handler, routeOptions)

	apiV2 := app.Group("/v2")
	v2API := openapi.New(app, apiV2, openapi.Config{Title: "tenant-pool-v2", Version: "test", Servers: []string{"/v2"}})
	midazhttp.InstallLedgerSchemaNamer(v2API)
	RegisterAccountV2RoutesToApp(apiV2, v2API, auth, handler, routeOptions)

	return &tenantPoolHarness{app: app, manager: manager, fake: fake, logs: logs, accountRepo: accountRepo}
}

// tenantPoolResponse is a parsed HTTP response.
type tenantPoolResponse struct {
	status int
	body   []byte
}

func (r tenantPoolResponse) field(t *testing.T, key string) any {
	t.Helper()

	var out map[string]any
	require.NoErrorf(t, json.Unmarshal(r.body, &out), "response is not JSON: %s", string(r.body))

	return out[key]
}

func (h *tenantPoolHarness) do(method, target, tenantID string, body any) (tenantPoolResponse, error) {
	var reader io.Reader

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return tenantPoolResponse{}, err
		}

		reader = strings.NewReader(string(raw))
	}

	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+compositionTenantJWT(tenantID))

	resp, err := h.app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		return tenantPoolResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return tenantPoolResponse{}, err
	}

	return tenantPoolResponse{status: resp.StatusCode, body: raw}, nil
}

func tenantPoolAccountsPath(version string, tn *tenantPoolTenant) string {
	return "/" + version + "/organizations/" + tn.orgID.String() + "/ledgers/" + tn.ledgerID.String() + "/accounts"
}

func (h *tenantPoolHarness) createAccount(version string, tn *tenantPoolTenant, alias string) (tenantPoolResponse, error) {
	return h.do(fiber.MethodPost, tenantPoolAccountsPath(version, tn), tn.id, map[string]any{
		"name":      "Account " + alias,
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
	})
}

func (h *tenantPoolHarness) getAccount(version string, tn *tenantPoolTenant, accountID string) (tenantPoolResponse, error) {
	return h.do(fiber.MethodGet, tenantPoolAccountsPath(version, tn)+"/"+accountID, tn.id, nil)
}

// closeTenantPool is the swap trigger: the tenant manager closes the tenant's pool
// without draining, as it does on a config change, a failed health check or a
// suspension. It detaches from the request's cancellation so the close is never
// issued on an already-cancelled context. It runs inside the request goroutine, so
// it reports with assert, never require.
func closeTenantPool(ctx context.Context, t *testing.T, manager *tmpostgres.Manager, tenantID string) {
	t.Helper()

	assert.NoError(t, manager.CloseConnection(context.WithoutCancel(ctx), tenantID))
}

// allowPlaintextPostgres lets libPostgres reach the plaintext testcontainer, both for
// the template migration and for the per-tenant pools the manager opens.
func allowPlaintextPostgres(t *testing.T) {
	t.Helper()

	t.Setenv("ALLOW_INSECURE_TLS", "true")
}

// countAccountsByAlias counts live accounts carrying alias in the given database.
func countAccountsByAlias(t *testing.T, tn *tenantPoolTenant, alias string) int {
	t.Helper()

	var n int

	require.NoError(t, tn.pg.DB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM account WHERE alias = $1 AND deleted_at IS NULL`, alias).Scan(&n))

	return n
}

// TestIntegration_TenantPoolResolution_NoSwapKeepsBehavior covers "Request sem troca de
// pool mantém o comportamento atual" and "Nenhum aviso sem troca": several PostgreSQL
// operations on an untouched pool succeed and log no swap warning.
func TestIntegration_TenantPoolResolution_NoSwapKeepsBehavior(t *testing.T) {
	allowPlaintextPostgres(t)

	tn := newTenantPoolTenant(t, "tenant-noswap")
	fake := newTenantPoolFake(t, tn.pg)
	fake.addTenant(tn.id, tn.pg.Config.DBName)

	h := newTenantPoolHarness(t, fake, tenantPoolHarnessConfig{withPoolResolution: true})

	created, err := h.createAccount("v2", tn, "@noswap")
	require.NoError(t, err)
	require.Equalf(t, fiber.StatusCreated, created.status, "create: %s", string(created.body))

	accountID, ok := created.field(t, "id").(string)
	require.True(t, ok, "created account must carry an id")

	got, err := h.getAccount("v2", tn, accountID)
	require.NoError(t, err)
	require.Equalf(t, fiber.StatusOK, got.status, "get: %s", string(got.body))
	assert.Equal(t, accountID, got.field(t, "id"))

	assert.Equal(t, 1, countAccountsByAlias(t, tn, "@noswap"))
	assert.Zero(t, h.logs.allSwapWarnings(), "no swap warning without a pool swap")
}

// TestIntegration_TenantPoolResolution_PoolSwappedBetweenOperations covers "Pool trocado
// entre duas operações da mesma request" (/v2), "Request em /v1 tem o mesmo
// comportamento" (/v1) and "Aviso emitido na troca": the tenant manager closes the pool
// after the ledger/asset/alias reads and before the account INSERT; the INSERT runs on
// the freshly resolved pool, the request answers 201 and the account is readable.
func TestIntegration_TenantPoolResolution_PoolSwappedBetweenOperations(t *testing.T) {
	allowPlaintextPostgres(t)

	for _, version := range []string{"v2", "v1"} {
		t.Run(version, func(t *testing.T) {
			tn := newTenantPoolTenant(t, "tenant-swap-"+version)
			fake := newTenantPoolFake(t, tn.pg)
			fake.addTenant(tn.id, tn.pg.Config.DBName)

			h := newTenantPoolHarness(t, fake, tenantPoolHarnessConfig{withPoolResolution: true})

			var closed atomic.Bool

			h.accountRepo.setBeforeCreate(func(ctx context.Context) {
				if closed.CompareAndSwap(false, true) {
					closeTenantPool(ctx, t, h.manager, tn.id)
				}
			})

			alias := "@swap-" + version

			created, err := h.createAccount(version, tn, alias)
			require.NoError(t, err)
			require.Equalf(t, fiber.StatusCreated, created.status, "create after pool swap: %s", string(created.body))
			require.True(t, closed.Load(), "the swap hook must have run")

			accountID, ok := created.field(t, "id").(string)
			require.True(t, ok, "created account must carry an id")

			assert.Equal(t, 1, h.logs.swapWarnings(tn.id), "exactly one swap warning: the INSERT is the only operation after the swap")

			got, err := h.getAccount(version, tn, accountID)
			require.NoError(t, err)
			require.Equalf(t, fiber.StatusOK, got.status, "get after pool swap: %s", string(got.body))
			assert.Equal(t, accountID, got.field(t, "id"))

			assert.Equal(t, 1, countAccountsByAlias(t, tn, alias))
		})
	}
}

// TestIntegration_TenantPoolResolution_TransactionOpenedBeforeCloseCommits covers
// "Transação aberta antes do fechamento do pool é confirmada": a transaction opened
// through the adapter keeps its connection after the pool is closed and commits on the
// original database, while the next adapter operation lands on the new pool. The fake
// repoints the tenant at a second database before the close, so the new pool is
// distinguishable and "nothing written to the new pool" is a real assertion.
func TestIntegration_TenantPoolResolution_TransactionOpenedBeforeCloseCommits(t *testing.T) {
	allowPlaintextPostgres(t)

	const tenantID = "tenant-tx"

	original := postgrestestutil.SetupMigratedContainer(t, "onboarding")
	replacement := postgrestestutil.SetupMigratedContainer(t, "onboarding")

	for _, pg := range []*postgrestestutil.ContainerResult{original, replacement} {
		_, err := pg.DB.ExecContext(context.Background(), `CREATE TABLE tenant_pool_probe (marker TEXT PRIMARY KEY)`)
		require.NoError(t, err)
	}

	fake := newTenantPoolFake(t, original)
	fake.addTenant(tenantID, original.Config.DBName)

	manager := newTenantPoolManager(t, newTenantPoolClient(t, fake))

	logs := &tenantPoolLogRecorder{}
	ctx := libObservability.ContextWithLogger(context.Background(), logs)

	pinned, err := manager.GetDB(ctx, tenantID)
	require.NoError(t, err)

	lazy := tenantpool.New(manager, tenantID, constant.ModuleOnboarding, pinned)

	tx, err := lazy.BeginTx(ctx, nil)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `INSERT INTO tenant_pool_probe (marker) VALUES ('in-tx-before-close')`)
	require.NoError(t, err)

	fake.pointTo(tenantID, replacement.Config.DBName)
	closeTenantPool(ctx, t, manager, tenantID)

	_, err = tx.ExecContext(ctx, `INSERT INTO tenant_pool_probe (marker) VALUES ('in-tx-after-close')`)
	require.NoError(t, err, "the open transaction keeps its connection after the pool is closed")
	require.NoError(t, tx.Commit(), "the commit completes on the original pool")

	_, err = lazy.ExecContext(ctx, `INSERT INTO tenant_pool_probe (marker) VALUES ('after-swap')`)
	require.NoError(t, err, "the next operation resolves the new pool")
	assert.Equal(t, 1, logs.swapWarnings(tenantID))

	markers := func(pg *postgrestestutil.ContainerResult) []string {
		rows, err := pg.DB.QueryContext(context.Background(), `SELECT marker FROM tenant_pool_probe ORDER BY marker`)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		var out []string

		for rows.Next() {
			var m string
			require.NoError(t, rows.Scan(&m))
			out = append(out, m)
		}

		require.NoError(t, rows.Err())

		return out
	}

	assert.Equal(t, []string{"in-tx-after-close", "in-tx-before-close"}, markers(original),
		"every transaction row is committed on the original database, and nothing else")
	assert.Equal(t, []string{"after-swap"}, markers(replacement),
		"the new pool received only the post-swap operation, no transaction row")
}

// TestIntegration_TenantPoolResolution_TenantSuspendedDuringRequest covers "Tenant suspenso
// durante a request": the pool is closed and the tenant manager no longer knows the
// tenant, so re-resolution fails and the INSERT runs on the closed pinned pool. The
// response must be the one the chain without the adapter gives today, and nothing is
// persisted.
func TestIntegration_TenantPoolResolution_TenantSuspendedDuringRequest(t *testing.T) {
	allowPlaintextPostgres(t)

	run := func(t *testing.T, withPoolResolution bool, tenantID string) (tenantPoolResponse, *tenantPoolHarness, *tenantPoolTenant) {
		t.Helper()

		tn := newTenantPoolTenant(t, tenantID)
		fake := newTenantPoolFake(t, tn.pg)
		fake.addTenant(tn.id, tn.pg.Config.DBName)

		h := newTenantPoolHarness(t, fake, tenantPoolHarnessConfig{withPoolResolution: withPoolResolution})

		var closed atomic.Bool

		h.accountRepo.setBeforeCreate(func(ctx context.Context) {
			if closed.CompareAndSwap(false, true) {
				fake.setMode(tn.id, tenantPoolFakeNotFound)
				closeTenantPool(ctx, t, h.manager, tn.id)
			}
		})

		resp, err := h.createAccount("v2", tn, "@suspended")
		require.NoError(t, err)
		require.True(t, closed.Load(), "the suspension hook must have run")

		return resp, h, tn
	}

	baseline, _, baselineTenant := run(t, false, "tenant-suspended-baseline")
	withAdapter, h, tn := run(t, true, "tenant-suspended")

	t.Logf("closed pool after suspension: without adapter %d code=%v, with adapter %d code=%v",
		baseline.status, baseline.field(t, "code"), withAdapter.status, withAdapter.field(t, "code"))

	require.GreaterOrEqualf(t, baseline.status, fiber.StatusInternalServerError,
		"baseline: a closed pool is a technical failure today: %s", string(baseline.body))
	assert.Equalf(t, baseline.status, withAdapter.status, "same status as today: %s", string(withAdapter.body))
	assert.Equal(t, baseline.field(t, "code"), withAdapter.field(t, "code"), "same error code as today")

	assert.Zero(t, countAccountsByAlias(t, baselineTenant, "@suspended"))
	assert.Zero(t, countAccountsByAlias(t, tn, "@suspended"), "nothing persisted after the suspension")
	assert.Zero(t, h.logs.allSwapWarnings(), "a failed re-resolution falls back to the pinned pool without a swap warning")
}

// TestIntegration_TenantPoolResolution_TenantManagerUnavailablePinnedOpen covers "Tenant
// manager indisponível e pool original ainda aberto": re-resolution fails while the
// pinned pool is still open, so every operation runs on the pinned pool and the request
// completes as it would without the outage.
//
// The real manager answers a cached pool without consulting the tenant manager, so a
// control-plane outage alone never makes GetDB fail here. The fake is made unavailable
// AND the adapter's Resolver is switched to fail, which is the condition the scenario
// describes: resolution fails, the pinned pool is untouched.
func TestIntegration_TenantPoolResolution_TenantManagerUnavailablePinnedOpen(t *testing.T) {
	allowPlaintextPostgres(t)

	tn := newTenantPoolTenant(t, "tenant-tm-down")
	fake := newTenantPoolFake(t, tn.pg)
	fake.addTenant(tn.id, tn.pg.Config.DBName)

	gated := &tenantPoolGatedResolver{}

	h := newTenantPoolHarness(t, fake, tenantPoolHarnessConfig{withPoolResolution: true, resolver: gated})
	gated.inner = h.manager

	h.accountRepo.setBeforeCreate(func(context.Context) {
		fake.setMode(tn.id, tenantPoolFakeUnavailable)
		gated.unavailable.Store(true)
	})

	created, err := h.createAccount("v2", tn, "@tm-down")
	require.NoError(t, err)
	require.Equalf(t, fiber.StatusCreated, created.status, "create with the tenant manager down: %s", string(created.body))
	require.Positive(t, gated.failures.Load(), "the INSERT must have tried, and failed, to re-resolve")

	accountID, ok := created.field(t, "id").(string)
	require.True(t, ok, "created account must carry an id")

	got, err := h.getAccount("v2", tn, accountID)
	require.NoError(t, err)
	require.Equalf(t, fiber.StatusOK, got.status, "get with the tenant manager down: %s", string(got.body))

	assert.Equal(t, 1, countAccountsByAlias(t, tn, "@tm-down"))
	assert.Zero(t, h.logs.allSwapWarnings(), "the pinned pool was used, so no swap warning")
}

// TestIntegration_TenantPoolResolution_ConcurrentTenantsIsolation covers "Troca de pool de
// um tenant durante requests concorrentes de dois tenants": every request of both tenants
// is parked right before its INSERT, the first tenant's pool is closed, and all are
// released together. Each tenant's accounts land only in its own database, the first
// tenant's requests complete on its new pool, and the second tenant is untouched.
func TestIntegration_TenantPoolResolution_ConcurrentTenantsIsolation(t *testing.T) {
	allowPlaintextPostgres(t)

	const perTenant = 3

	tenantA := newTenantPoolTenant(t, "tenant-iso-a")
	tenantB := newTenantPoolTenant(t, "tenant-iso-b")

	fake := newTenantPoolFake(t, tenantA.pg)
	fake.addTenant(tenantA.id, tenantA.pg.Config.DBName)
	fake.addTenant(tenantB.id, tenantB.pg.Config.DBName)

	h := newTenantPoolHarness(t, fake, tenantPoolHarnessConfig{withPoolResolution: true})

	// Each request reports its arrival at the INSERT on a buffered channel (never
	// blocks) and parks until release. The coordinator is the only waiter: it
	// drains the arrivals under one deadline, closes tenant A's pool, and always
	// closes release, so no goroutine outlives the test even when a request fails
	// before reaching the hook.
	const totalRequests = 2 * perTenant

	arrivals := make(chan struct{}, totalRequests)
	release := make(chan struct{})

	h.accountRepo.setBeforeCreate(func(ctx context.Context) {
		arrivals <- struct{}{}

		select {
		case <-release:
		case <-ctx.Done():
		}
	})

	closer := make(chan error, 1)

	go func() {
		defer close(release)

		deadline := time.NewTimer(tenantPoolRequestTimeout)
		defer deadline.Stop()

		for arrived := 0; arrived < totalRequests; arrived++ {
			select {
			case <-arrivals:
			case <-deadline.C:
				closer <- fmt.Errorf("only %d of %d requests reached the INSERT within %s", arrived, totalRequests, tenantPoolRequestTimeout)

				return
			}
		}

		closer <- h.manager.CloseConnection(context.Background(), tenantA.id)
	}()

	var g errgroup.Group

	for _, tn := range []*tenantPoolTenant{tenantA, tenantB} {
		for i := 0; i < perTenant; i++ {
			alias := fmt.Sprintf("@iso-%s-%d", tn.id, i)

			g.Go(func() error {
				resp, err := h.createAccount("v2", tn, alias)
				if err != nil {
					return err
				}

				if resp.status != fiber.StatusCreated {
					return fmt.Errorf("%s create %s got %d: %s", tn.id, alias, resp.status, string(resp.body))
				}

				return nil
			})
		}
	}

	require.NoError(t, g.Wait(), "every request of both tenants answers 201")
	require.NoError(t, <-closer, "the first tenant's pool must have been closed while every request was parked")

	for i := 0; i < perTenant; i++ {
		aliasA := fmt.Sprintf("@iso-%s-%d", tenantA.id, i)
		aliasB := fmt.Sprintf("@iso-%s-%d", tenantB.id, i)

		assert.Equalf(t, 1, countAccountsByAlias(t, tenantA, aliasA), "%s in tenant A's database", aliasA)
		assert.Equalf(t, 1, countAccountsByAlias(t, tenantB, aliasB), "%s in tenant B's database", aliasB)
		assert.Zerof(t, countAccountsByAlias(t, tenantB, aliasA), "%s must not reach tenant B's database", aliasA)
		assert.Zerof(t, countAccountsByAlias(t, tenantA, aliasB), "%s must not reach tenant A's database", aliasB)
	}

	assert.Equal(t, perTenant, h.logs.swapWarnings(tenantA.id), "one swap warning per tenant A request")
	assert.Zero(t, h.logs.swapWarnings(tenantB.id), "tenant B's pool was never swapped")
}
