// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

// This file is the shared harness for the P4 third-rail fee proof suite. The
// proof classes themselves live in transaction_fee_proof_integration_test.go
// (T16), transaction_fee_revert_integration_test.go (T14), and
// transaction_fee_async_integration_test.go (T25). The harness wires a
// fee-enabled TransactionHandler against real Postgres + Mongo + Redis (and, for
// the async file, RabbitMQ) by reusing the production composition: the same
// command/query/fees use cases the unified ledger bootstrap builds at
// config.go:798 (transactionHandler := &TransactionHandler{Command, Query,
// FeeApplier: fees.useCase}).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongoonb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	mongotxn "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feesservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	feemodel "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	authMiddleware "github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"

	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feeHarness holds a fully-wired fee-enabled transaction stack backed by real
// containers, with the org/ledger seeded into the onboarding schema so the fee
// engine's in-process query-layer resolver works exactly as in production.
type feeHarness struct {
	pgContainer    *postgrestestutil.ContainerResult
	mongoContainer *mongotestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult

	pgConn      *libPostgres.Client
	db          *sql.DB
	redisRepo   redis.RedisRepository
	metaRepo    mongotxn.Repository
	packageRepo pack.Repository

	commandUC *command.UseCase
	queryUC   *query.UseCase
	feeUC     *feesservices.UseCase
	handler   *TransactionHandler

	orgID    uuid.UUID
	ledgerID uuid.UUID
}

// setupFeeHarness builds the fee-enabled stack. It mirrors the production
// composition root: a single command.UseCase + query.UseCase share the
// onboarding + transaction Postgres repos, the transaction Mongo metadata repo,
// and the Redis repo; the fee use case is built from the fee Mongo package repo
// and an in-process MidazResolver over the same query.UseCase, and injected as
// the handler's FeeApplier — the seam exercised by executeCreateTransaction.
//
// RabbitMQ is intentionally absent: the default sync path persists inline, which
// is what every proof class except the T25 async file needs. The async file
// builds its own RabbitMQ-backed variant.
func setupFeeHarness(t *testing.T) *feeHarness {
	t.Helper()

	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	h := &feeHarness{}

	h.pgContainer = postgrestestutil.SetupContainer(t)
	h.mongoContainer = mongotestutil.SetupContainer(t)
	h.redisContainer = redistestutil.SetupContainer(t)
	h.db = h.pgContainer.DB

	// Transaction schema via golang-migrate (owns schema_migrations).
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(h.pgContainer.Host, h.pgContainer.Port, h.pgContainer.Config)
	h.pgConn = postgrestestutil.CreatePostgresClient(t, connStr, connStr, h.pgContainer.Config.DBName, migrationsPath)

	// Onboarding schema applied directly (disjoint tables; IF NOT EXISTS).
	postgrestestutil.ApplyOnboardingSchema(t, h.db)

	mongoTxnConn := mongotestutil.CreateConnection(t, h.mongoContainer.URI, "test_db")
	redisConn := redistestutil.CreateConnection(t, h.redisContainer.Addr)

	// Transaction-domain repos.
	transactionRepo := transaction.NewTransactionPostgreSQLRepository(h.pgConn)
	operationRepo := operation.NewOperationPostgreSQLRepository(h.pgConn)
	balanceRepo := balance.NewBalancePostgreSQLRepository(h.pgConn, false)
	h.metaRepo = mongotxn.NewMetadataMongoDBRepository(mongoTxnConn)

	redisRepo, err := redis.NewConsumerRedis(redisConn, unblockedAccountsSource{})
	require.NoError(t, err, "redis repo")
	h.redisRepo = redisRepo

	// Onboarding-domain repos (needed by the fee resolver's account/segment reads
	// and by GetParsedLedgerSettings on the create funnel).
	orgRepo := organization.NewOrganizationPostgreSQLRepository(h.pgConn)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(h.pgConn)
	assetRepo := asset.NewAssetPostgreSQLRepository(h.pgConn)
	accountRepo := account.NewAccountPostgreSQLRepository(h.pgConn)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(h.pgConn)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(h.pgConn)
	onbMetaRepo := mongoonb.NewMetadataMongoDBRepository(mongoTxnConn)

	h.queryUC = &query.UseCase{
		OrganizationRepo:        orgRepo,
		LedgerRepo:              ledgerRepo,
		AssetRepo:               assetRepo,
		AccountRepo:             accountRepo,
		PortfolioRepo:           portfolioRepo,
		SegmentRepo:             segmentRepo,
		OnboardingMetadataRepo:  onbMetaRepo,
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		BalanceRepo:             balanceRepo,
		TransactionMetadataRepo: h.metaRepo,
		TransactionRedisRepo:    redisRepo,
	}
	h.commandUC = &command.UseCase{
		OrganizationRepo:        orgRepo,
		LedgerRepo:              ledgerRepo,
		AssetRepo:               assetRepo,
		AccountRepo:             accountRepo,
		PortfolioRepo:           portfolioRepo,
		SegmentRepo:             segmentRepo,
		OnboardingMetadataRepo:  onbMetaRepo,
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		BalanceRepo:             balanceRepo,
		TransactionMetadataRepo: h.metaRepo,
		TransactionRedisRepo:    redisRepo,
	}

	// Fee Mongo: inject the already-connected container client so the repo's
	// GetDB + EnsureIndexes run against real Mongo without re-dialing.
	logger := &libLog.GoLogger{}
	feeConn := &feesmongo.MongoConnection{
		ConnectionStringSource: h.mongoContainer.URI,
		Database:               "test_db",
		MaxPoolSize:            1,
		DB:                     h.mongoContainer.Client,
	}
	packageRepo, err := pack.NewPackageMongoDBRepository(feeConn, logger)
	require.NoError(t, err, "fee package repo")
	h.packageRepo = packageRepo

	resolver, err := feesservices.NewQueryResolver(h.queryUC)
	require.NoError(t, err, "fee resolver")
	h.feeUC, err = feesservices.NewUseCase(packageRepo, resolver)
	require.NoError(t, err, "fee use case")

	h.handler = &TransactionHandler{Query: h.queryUC, Command: h.commandUC, FeeApplier: h.feeUC}

	// Seed a real organization + ledger so GetParsedLedgerSettings succeeds and
	// the fee resolver resolves accounts against a real ledger.
	h.orgID = postgrestestutil.CreateTestOrganization(t, h.db)
	h.ledgerID = postgrestestutil.CreateTestLedger(t, h.db, h.orgID)

	return h
}

// dropFeePrecisionTable is a no-op assertion that the ISO-4217 precision table
// (asset_precision) does not exist in this schema, proving proof class 3 runs
// "with the precision table deleted" (P4-T11). The fee engine emits unrounded
// legs and reconciles residuals onto the max account; no precision table is
// consulted. We assert its absence so a future reintroduction is caught.
func (h *feeHarness) assertNoPrecisionTable(t *testing.T) {
	t.Helper()

	var exists bool
	err := h.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'asset_precision')`).Scan(&exists)
	require.NoError(t, err, "query for asset_precision table")
	require.False(t, exists, "the ISO-4217 asset_precision table must NOT exist (P4-T11 deleted it); residual reconciliation alone holds the balance")
}

// ctx returns a background context for fixture seeding outside the request path.
func (h *feeHarness) ctx() context.Context { return context.Background() }

// =============================================================================
// App wiring, request helpers, seeding
// =============================================================================

// ─── HTTP app wiring ─────────────────────────────────────────────────────────

// debugFunnelLogs, when true, injects a Debug-level logger into each request so
// the create funnel's own Debug logs (balance-op counts, etc.) surface in test
// output. Toggled on only while diagnosing.
var debugFunnelLogs = false

// newApp builds a Fiber app exposing the /v1 transaction routes, mounted through
// RegisterTransactionRoutes exactly as production does: ParseUUIDPathParameters runs as
// Fiber middleware on the /v1 group and the Huma registrar owns the terminals, so body
// decode/validate goes through the same http.DecodeAndValidate pipeline as production.
//
// The /v1 contract carries no fee engine, so this app serves the proofs that a create
// posts exactly as authored. The proofs that fees ARE charged drive newV2App.
func (h *feeHarness) newApp() *fiber.App {
	app := fiber.New()

	libProblem.Install()
	http.InstallHumaFrameworkErrors()

	// Mirror production: the ledger registers ErrorEnvelope on the app root, so
	// /v1 serves the v3 envelope. Without it these assertions lock a shape no
	// deployed ledger returns.
	app.Use(ledgerMiddleware.ErrorEnvelope())

	apiV1 := app.Group("/v1")
	hAPI := openapi.New(app, apiV1, openapi.Config{
		Title:   "ledger-fee-integration",
		Version: "test",
		Servers: []string{"/v1"},
	})

	// The transaction Out nests operation.{Status,Balance,Amount}, which collide on
	// bare schema names with the mmodel/transaction types on the shared registry.
	// Must run after openapi.New and BEFORE any huma.Register.
	http.InstallLedgerSchemaNamer(hAPI)

	debugLogger := func(c fiber.Ctx) error {
		if debugFunnelLogs {
			c.SetContext(libObservability.ContextWithLogger(c.Context(), &libLog.GoLogger{Level: libLog.LevelDebug}))
		}

		return c.Next()
	}

	mountTransactionRoutes(apiV1, debugLogger)

	RegisterTransactionRoutes(hAPI, h.handler)

	return app
}

// txResponse captures the parsed HTTP response from a create/state call.
type txResponse struct {
	status   int
	rawBody  []byte
	body     map[string]any
	replayed string
}

// post issues a JSON POST and parses the response.
func (h *feeHarness) post(t *testing.T, app *fiber.App, path, body string, headers map[string]string) txResponse {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "HTTP request failed")

	rb, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	_ = resp.Body.Close()

	out := txResponse{status: resp.StatusCode, rawBody: rb, replayed: resp.Header.Get("X-Idempotency-Replayed")}
	_ = json.Unmarshal(rb, &out.body)

	return out
}

// createJSON drives a transactions/json create and returns the response.
func (h *feeHarness) createJSON(t *testing.T, app *fiber.App, body string, headers map[string]string) txResponse {
	t.Helper()
	return h.post(t, app, h.txPath("json"), body, headers)
}

// txPath builds the create path for a given mode.
func (h *feeHarness) txPath(mode string) string {
	return "/v1/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() + "/transactions/" + mode
}

// statePath builds a commit/cancel/revert path for a transaction.
func (h *feeHarness) statePath(txID uuid.UUID, action string) string {
	return "/v1/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() + "/transactions/" + txID.String() + "/" + action
}

// ─── /v2 HTTP app wiring ─────────────────────────────────────────────────────

// The fee engine is a /v2 contract: /v1 posts a transaction exactly as authored, so every
// proof that fees ARE charged drives the /v2 app below, and newApp above stays mounted for
// the /v1 proofs that they are NOT. Both apps share the same handler, seeding, and leg
// assertions — only the route version and the request wire shape differ.

// newV2App builds a Fiber app exposing the /v2 transaction routes, mounted through
// RegisterTransactionV2RoutesToApp exactly as production does: the registrar owns both the
// Fiber guard chain and the Huma terminals, so body decode/validate goes through the same
// http.DecodeAndValidate pipeline as production.
func (h *feeHarness) newV2App() *fiber.App {
	app := fiber.New()

	libProblem.Install()
	http.InstallHumaFrameworkErrors()

	// Mirror production: /v2 serves the RFC 9457 problem envelope through the same
	// root-mounted ErrorEnvelope the ledger registers.
	app.Use(ledgerMiddleware.ErrorEnvelope())

	if debugFunnelLogs {
		app.Use(func(c fiber.Ctx) error {
			c.SetContext(libObservability.ContextWithLogger(c.Context(), &libLog.GoLogger{Level: libLog.LevelDebug}))

			return c.Next()
		})
	}

	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{
		Title:   "ledger-fee-integration-v2",
		Version: "test",
		Servers: []string{"/v2"},
	})

	// Same shared-registry collision the /v1 app avoids; must run after openapi.New and
	// BEFORE any huma.Register.
	http.InstallLedgerSchemaNamer(hAPI)

	RegisterTransactionV2RoutesToApp(apiV2, hAPI, &authMiddleware.AuthClient{Enabled: false}, h.handler, nil)

	return app
}

// v2Scope spells the harness org/ledger for a v2 leg. A /v2 create URL names neither, so
// the scope a request is posted against travels in every leg of the body.
func (h *feeHarness) v2Scope() string {
	return `"organizationId":"` + h.orgID.String() + `","ledgerId":"` + h.ledgerID.String() + `"`
}

// v2Leg builds one flat v2 debit/credit leg against the harness scope.
func (h *feeHarness) v2Leg(alias, amount string) string {
	return `{"alias":"` + alias + `",` + h.v2Scope() + `,"amount":"` + amount + `"}`
}

// v2Body assembles a flat v2 create body from already-built legs.
func (h *feeHarness) v2Body(description, asset, amount string, debits, credits []string) string {
	return `{"description":"` + description + `","asset":"` + asset + `","amount":"` + amount + `"` +
		`,"debits":[` + strings.Join(debits, ",") + `]` +
		`,"credits":[` + strings.Join(credits, ",") + `]}`
}

// v2CreatePath builds the create path for a v2 action (direct, hold, block, unblock).
func (h *feeHarness) v2CreatePath(action string) string {
	return "/v2/transactions/" + action
}

// v2StatePath builds a v2 commit/cancel/revert path.
func (h *feeHarness) v2StatePath(txID uuid.UUID, action string) string {
	return "/v2/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() +
		"/transactions/" + txID.String() + "/" + action
}

// createV2Direct drives the v2 direct (non-pending) create and returns the response.
func (h *feeHarness) createV2Direct(t *testing.T, app *fiber.App, body string, headers map[string]string) txResponse {
	t.Helper()

	return h.post(t, app, h.v2CreatePath("direct"), body, headers)
}

// createV2Hold drives the v2 hold (pending) create and returns the response.
func (h *feeHarness) createV2Hold(t *testing.T, app *fiber.App, body string, headers map[string]string) txResponse {
	t.Helper()

	return h.post(t, app, h.v2CreatePath("hold"), body, headers)
}

// ─── balance seeding ─────────────────────────────────────────────────────────

// seedBalance creates a balance row and a matching ACTIVE account so the fee
// resolver's GetAccountByAlias finds it. Returns the balance row ID.
func (h *feeHarness) seedBalance(t *testing.T, alias, asset string, available decimal.Decimal, accountType string) uuid.UUID {
	t.Helper()

	accParams := postgrestestutil.DefaultAccountParams()
	accParams.Alias = alias
	accParams.AssetCode = asset
	accParams.Type = accountType
	accountID := postgrestestutil.CreateTestAccountWithParams(t, h.db, h.orgID, h.ledgerID, accParams)

	balParams := postgrestestutil.DefaultBalanceParams()
	balParams.Alias = alias
	balParams.AssetCode = asset
	balParams.Available = available
	balParams.OnHold = decimal.Zero
	balParams.AccountType = accountType

	return postgrestestutil.CreateTestBalance(t, h.db, h.orgID, h.ledgerID, accountID, balParams)
}

// seedBalanceWithSegment is like seedBalance but assigns the account a segment.
func (h *feeHarness) seedBalanceWithSegment(t *testing.T, alias, asset string, available decimal.Decimal, segmentID uuid.UUID) uuid.UUID {
	t.Helper()

	accParams := postgrestestutil.DefaultAccountParams()
	accParams.Alias = alias
	accParams.AssetCode = asset
	accParams.Type = "deposit"
	accParams.SegmentID = &segmentID
	accountID := postgrestestutil.CreateTestAccountWithParams(t, h.db, h.orgID, h.ledgerID, accParams)

	balParams := postgrestestutil.DefaultBalanceParams()
	balParams.Alias = alias
	balParams.AssetCode = asset
	balParams.Available = available
	balParams.AccountType = "deposit"

	return postgrestestutil.CreateTestBalance(t, h.db, h.orgID, h.ledgerID, accountID, balParams)
}

// ─── package seeding ─────────────────────────────────────────────────────────

// feeSpec describes one fee inside a seeded package.
type feeSpec struct {
	label         string
	rule          string // "flatFee" | "percentual" | "maxBetweenTypes"
	calcs         []feemodel.Calculation
	deductible    bool
	creditAccount string
	priority      int
	referenceAmt  string // defaults to originalAmount
}

// packageSpec describes a fee package to seed.
type packageSpec struct {
	label          string
	minAmount      decimal.Decimal
	maxAmount      decimal.Decimal
	segmentID      *uuid.UUID
	waivedAccounts []string
	fees           []feeSpec
}

// seedPackage persists a package from the spec via the real repository and
// returns its ID.
func (h *feeHarness) seedPackage(t *testing.T, spec packageSpec) uuid.UUID {
	t.Helper()

	enable := true
	fees := make(map[string]feemodel.Fee, len(spec.fees))

	for i, f := range spec.fees {
		ded := f.deductible
		ref := f.referenceAmt
		if ref == "" {
			ref = "originalAmount"
		}
		priority := f.priority
		if priority == 0 {
			priority = i + 1
		}

		key := f.label
		if key == "" {
			key = "fee_" + decimal.NewFromInt(int64(i)).String()
		}

		fees[key] = feemodel.Fee{
			FeeLabel: f.label,
			CalculationModel: &feemodel.CalculationModel{
				ApplicationRule: f.rule,
				Calculations:    f.calcs,
			},
			ReferenceAmount:  ref,
			Priority:         priority,
			IsDeductibleFrom: &ded,
			CreditAccount:    f.creditAccount,
		}
	}

	maxAmt := spec.maxAmount
	if maxAmt.IsZero() {
		maxAmt = decimal.NewFromInt(1_000_000_000)
	}

	p, err := pack.NewPackage(h.orgID, h.ledgerID, spec.label, spec.minAmount, maxAmt, fees, &enable)
	require.NoError(t, err, "build package")

	p.SegmentID = spec.segmentID
	if len(spec.waivedAccounts) > 0 {
		wa := spec.waivedAccounts
		p.WaivedAccounts = &wa
	}

	created, err := h.packageRepo.Create(h.ctx(), p, h.orgID)
	require.NoError(t, err, "persist package")

	return created.ID
}

// flatFee builds a flatFee fee spec.
func flatFee(label, creditAccount, value string, deductible bool) feeSpec {
	return feeSpec{
		label:         label,
		rule:          "flatFee",
		calcs:         []feemodel.Calculation{{Type: "flat", Value: value}},
		deductible:    deductible,
		creditAccount: creditAccount,
	}
}

// percentualFee builds a percentual fee spec.
func percentualFee(label, creditAccount, percent string, deductible bool) feeSpec {
	return feeSpec{
		label:         label,
		rule:          "percentual",
		calcs:         []feemodel.Calculation{{Type: "percentage", Value: percent}},
		deductible:    deductible,
		creditAccount: creditAccount,
	}
}

// maxBetweenFee builds a maxBetweenTypes fee spec with a flat and a percentage leg.
func maxBetweenFee(label, creditAccount, flatVal, percentVal string, deductible bool) feeSpec {
	return feeSpec{
		label: label,
		rule:  "maxBetweenTypes",
		calcs: []feemodel.Calculation{
			{Type: "flat", Value: flatVal},
			{Type: "percentage", Value: percentVal},
		},
		deductible:    deductible,
		creditAccount: creditAccount,
	}
}

// ─── persisted operation legs ────────────────────────────────────────────────

// persistedLeg is one row of the Postgres operation table for a transaction.
type persistedLeg struct {
	Type   string
	Alias  string
	Amount decimal.Decimal
	Key    string
	Route  *string
}

// loadLegs reads all operations persisted for a transaction.
func loadLegs(t *testing.T, db *sql.DB, txID uuid.UUID) []persistedLeg {
	t.Helper()

	rows, err := db.Query(`SELECT type, account_alias, amount, balance_key, route FROM operation WHERE transaction_id = $1`, txID)
	require.NoError(t, err, "query operations")
	defer func() { _ = rows.Close() }()

	var legs []persistedLeg
	for rows.Next() {
		var l persistedLeg
		require.NoError(t, rows.Scan(&l.Type, &l.Alias, &l.Amount, &l.Key, &l.Route), "scan operation")
		legs = append(legs, l)
	}
	require.NoError(t, rows.Err(), "operation rows iteration")

	return legs
}

// signedSum computes the signed sum of legs under the double-entry convention:
// CREDIT is positive, DEBIT/ON_HOLD is negative. A balanced transaction nets to
// exactly zero under decimal.Equal.
func signedSum(legs []persistedLeg) decimal.Decimal {
	sum := decimal.Zero
	for _, l := range legs {
		switch l.Type {
		case "CREDIT":
			sum = sum.Add(l.Amount)
		case "DEBIT", "ON_HOLD":
			sum = sum.Sub(l.Amount)
		}
	}
	return sum
}

// requireBalanced asserts the legs net to zero under EXACT decimal equality.
func requireBalanced(t *testing.T, legs []persistedLeg, msg string) {
	t.Helper()
	sum := signedSum(legs)
	require.Truef(t, sum.Equal(decimal.Zero), "%s: legs must net to exactly zero, got %s", msg, sum.String())
}

// sumAmounts sums the absolute amounts of the given legs.
func sumAmounts(legs []persistedLeg) decimal.Decimal {
	sum := decimal.Zero
	for _, l := range legs {
		sum = sum.Add(l.Amount)
	}
	return sum
}

// dbTxStatus reads the persisted transaction status.
func dbTxStatus(t *testing.T, db *sql.DB, txID uuid.UUID) string {
	t.Helper()
	return postgrestestutil.GetTransactionStatus(t, db, txID)
}

// dbTxAmount reads the persisted transaction amount.
func dbTxAmount(t *testing.T, db *sql.DB, txID uuid.UUID) decimal.Decimal {
	t.Helper()
	var amt decimal.Decimal
	err := db.QueryRow(`SELECT amount FROM transaction WHERE id = $1`, txID).Scan(&amt)
	require.NoError(t, err, "read transaction amount")
	return amt
}

// approvedStatus is a convenience alias used across proof assertions.
const approvedStatus = cn.APPROVED

// =============================================================================
// Harness sanity
// =============================================================================

// TestFeeHarness_Sanity_NoPackageSucceeds proves the harness itself is sound:
// with NO fee package seeded, applyFees is a no-op and a plain transfer creates
// and balances exactly like the existing non-fee integration suite. This
// isolates the proof-suite failures to the fee seam (when a package IS present),
// not to the harness wiring.
func TestFeeHarness_Sanity_NoPackageSucceeds(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newV2App()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(1000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

	// No package seeded -> the fee engine finds no package -> applyFees is a no-op even
	// on the /v2 contract that reaches it.
	body := h.v2Body("no-fee transfer through the fee harness", "USD", "100",
		[]string{h.v2Leg("@payer", "100")},
		[]string{h.v2Leg("@receiver", "100")})

	resp := h.createV2Direct(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "no-fee create through the harness must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))

	legs := loadLegs(t, h.db, txID)
	require.Len(t, legs, 2, "a plain transfer must persist exactly 2 operations")
	requireBalanced(t, legs, "no-fee transfer")
	assert.True(t, dbTxAmount(t, h.db, txID).Equal(decimal.NewFromInt(100)))
}

// legsFor returns the legs booked against alias. An empty legType matches any
// operation type; otherwise only legs of that type are returned.
func legsFor(legs []persistedLeg, alias, legType string) []persistedLeg {
	var out []persistedLeg

	for _, l := range legs {
		if l.Alias == alias && (legType == "" || l.Type == legType) {
			out = append(out, l)
		}
	}

	return out
}

// mustTxID extracts the transaction id from a successful create response.
func mustTxID(t *testing.T, resp txResponse) uuid.UUID {
	t.Helper()
	idStr, ok := resp.body["id"].(string)
	require.Truef(t, ok, "response must contain id: %s", string(resp.rawBody))
	id, err := uuid.Parse(idStr)
	require.NoError(t, err, "transaction id must be a valid UUID")
	return id
}
