// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	onbRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/onboarding"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockUnblockInfra extends the read-path integration harness with an
// OperationHandler (for the account operations list) and the transaction list
// route, so the BLOCK/UNBLOCK type can be asserted end-to-end across every read
// surface against a real Postgres + Mongo + Redis stack.
type blockUnblockInfra struct {
	pgContainer     *postgrestestutil.ContainerResult
	mongoContainer  *mongotestutil.ContainerResult
	redisContainer  *redistestutil.ContainerResult
	onboardingRedis onbRedis.RedisRepository
	txHandler       *TransactionHandler
	opHandler       *OperationHandler
	app             *fiber.App
	orgID           uuid.UUID
	ledgerID        uuid.UUID
}

// setupBlockUnblockInfra mirrors setupTestInfra (sync mode, no RabbitMQ) and
// additionally wires the read paths exercised by this task:
//   - POST .../transactions/block
//   - POST .../transactions/unblock
//   - GET  .../transactions/{transaction_id}        (single)
//   - GET  .../transactions                          (list)
//   - GET  .../accounts/{account_id}/operations      (account operations)
func setupBlockUnblockInfra(t *testing.T) *blockUnblockInfra {
	t.Helper()

	// Sync mode: no RabbitMQ container, no async features.
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	infra := &blockUnblockInfra{}

	infra.pgContainer = postgrestestutil.SetupContainer(t)
	infra.mongoContainer = mongotestutil.SetupContainer(t)
	infra.redisContainer = redistestutil.SetupContainer(t)

	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(infra.pgContainer.Host, infra.pgContainer.Port, infra.pgContainer.Config)
	pgConn := postgrestestutil.CreatePostgresClient(t, connStr, connStr, infra.pgContainer.Config.DBName, migrationsPath)

	mongoConn := mongotestutil.CreateConnection(t, infra.mongoContainer.URI, "test_db")
	redisConn := redistestutil.CreateConnection(t, infra.redisContainer.Addr)

	transactionRepo := transaction.NewTransactionPostgreSQLRepository(pgConn)
	operationRepo := operation.NewOperationPostgreSQLRepository(pgConn)
	balanceRepo := balance.NewBalancePostgreSQLRepository(pgConn)
	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongoConn)
	redisRepo, err := redis.NewConsumerRedis(redisConn)
	require.NoError(t, err, "failed to create Redis repository")

	// The create path resolves ledger settings via the onboarding Redis cache
	// (query.UseCase.GetParsedLedgerSettings). The ledger table lives in the
	// onboarding component, not the transaction DB this harness connects to, so
	// we wire an onboarding Redis repository against the same container and
	// pre-seed the settings cache (see seedLedgerSettings). On a cache hit the
	// settings lookup never falls through to the (absent) ledger DB.
	onboardingRedisRepo, err := onbRedis.NewConsumerRedis(redisConn)
	require.NoError(t, err, "failed to create onboarding Redis repository")
	infra.onboardingRedis = onboardingRedisRepo

	queryUC := &query.UseCase{
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		BalanceRepo:             balanceRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
		OnboardingRedisRepo:     onboardingRedisRepo,
	}
	commandUC := &command.UseCase{
		TransactionRepo:         transactionRepo,
		OperationRepo:           operationRepo,
		BalanceRepo:             balanceRepo,
		TransactionMetadataRepo: metadataRepo,
		TransactionRedisRepo:    redisRepo,
	}

	infra.txHandler = &TransactionHandler{Query: queryUC, Command: commandUC}
	infra.opHandler = &OperationHandler{Query: queryUC, Command: commandUC}

	// org/ledger live in the onboarding component; the transaction component
	// only echoes these IDs back, so fakes are sufficient.
	infra.orgID = uuid.Must(libCommons.GenerateUUIDv7())
	infra.ledgerID = uuid.Must(libCommons.GenerateUUIDv7())

	infra.app = fiber.New()
	infra.setupRoutes()

	return infra
}

func (infra *blockUnblockInfra) setupRoutes() {
	// parseParam resolves a UUID path parameter into c.Locals. A malformed value
	// surfaces as an HTTP 400 instead of silently becoming the zero UUID (which
	// would mask a routing/derivation bug behind a confusing not-found later).
	parseParam := func(c *fiber.Ctx, name string) error {
		v := c.Params(name)
		if v == "" {
			return nil
		}

		id, err := uuid.Parse(v)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).
				JSON(fiber.Map{"error": "invalid path parameter " + name + ": " + v})
		}

		c.Locals(name, id)

		return nil
	}

	paramMiddleware := func(c *fiber.Ctx) error {
		for _, name := range []string{"organization_id", "ledger_id", "transaction_id", "account_id"} {
			if err := parseParam(c, name); err != nil {
				return err
			}
		}

		return c.Next()
	}

	base := "/v1/organizations/:organization_id/ledgers/:ledger_id"

	infra.app.Post(base+"/transactions/block",
		paramMiddleware, http.WithBody(new(mtransaction.CreateTransactionInput), infra.txHandler.CreateTransactionBlock))
	infra.app.Post(base+"/transactions/unblock",
		paramMiddleware, http.WithBody(new(mtransaction.CreateTransactionInput), infra.txHandler.CreateTransactionUnblock))
	infra.app.Get(base+"/transactions/:transaction_id",
		paramMiddleware, infra.txHandler.GetTransaction)
	infra.app.Get(base+"/transactions",
		paramMiddleware, infra.txHandler.GetAllTransactions)
	infra.app.Get(base+"/accounts/:account_id/operations",
		paramMiddleware, infra.opHandler.GetAllOperationsByAccount)
}

// seedLedgerSettings pre-populates the onboarding Redis settings cache so the
// create path's GetParsedLedgerSettings resolves on a cache hit. An empty
// settings map is valid: ParseLedgerSettings applies defaults (route and
// account-type validation disabled), keeping these direct transfers on the
// happy path.
func (infra *blockUnblockInfra) seedLedgerSettings(t *testing.T) {
	t.Helper()

	key := utils.LedgerSettingsInternalKey(infra.orgID, infra.ledgerID)
	require.NoError(t,
		infra.onboardingRedis.Set(context.Background(), key, "{}", query.SettingsCacheTTL),
		"should seed ledger settings cache")
}

// createTransfer issues a typed transfer (BLOCK or UNBLOCK) of `value` USD from
// sourceAlias to destAlias and returns the resulting transaction ID.
func (infra *blockUnblockInfra) createTransfer(t *testing.T, endpoint, sourceAlias, destAlias, value string) uuid.UUID {
	t.Helper()

	body := `{
		"description": "block/unblock read-path integration",
		"send": {
			"asset": "USD",
			"value": "` + value + `",
			"source": { "from": [ { "accountAlias": "` + sourceAlias + `", "amount": { "asset": "USD", "value": "` + value + `" } } ] },
			"distribute": { "to": [ { "accountAlias": "` + destAlias + `", "amount": { "asset": "USD", "value": "` + value + `" } } ] }
		}
	}`

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+endpoint,
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "create request should not fail")
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read create response body")

	require.Equal(t, 201, resp.StatusCode,
		"expected HTTP 201 for %s, got %d: %s", endpoint, resp.StatusCode, string(respBody))

	var created map[string]any
	require.NoError(t, json.Unmarshal(respBody, &created), "create response should be valid JSON")

	idStr, ok := created["id"].(string)
	require.True(t, ok, "create response should contain transaction id")

	id, err := uuid.Parse(idStr)
	require.NoError(t, err, "transaction id should be a valid UUID")

	return id
}

// getJSON performs a GET against the app and decodes the JSON body.
func (infra *blockUnblockInfra) getJSON(t *testing.T, path string) map[string]any {
	t.Helper()

	req := httptest.NewRequest("GET",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+path, nil)

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "GET %s should not fail", path)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read GET %s response body", path)

	require.Equal(t, 200, resp.StatusCode,
		"expected HTTP 200 for GET %s, got %d: %s", path, resp.StatusCode, string(body))

	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out), "GET %s response should be valid JSON", path)

	return out
}

// operationTypesByTransaction reads the persisted operation rows for a
// transaction directly from Postgres and returns their type/direction pairs.
func operationTypesByTransaction(t *testing.T, infra *blockUnblockInfra, txID uuid.UUID) []struct{ Type, Direction string } {
	t.Helper()

	rows, err := infra.pgContainer.DB.Query(
		`SELECT type, direction FROM operation WHERE transaction_id = $1 ORDER BY direction`, txID,
	)
	require.NoError(t, err, "should query operation rows")
	defer rows.Close()

	var result []struct{ Type, Direction string }

	for rows.Next() {
		var typ, dir string
		require.NoError(t, rows.Scan(&typ, &dir), "should scan operation row")
		result = append(result, struct{ Type, Direction string }{typ, dir})
	}

	require.NoError(t, rows.Err(), "should iterate operation rows without error")

	return result
}

// containsString reports whether target appears in the array under key, where
// the array comes from a decoded JSON object (i.e. []any of strings).
func arrayContains(t *testing.T, obj map[string]any, key, target string) bool {
	t.Helper()

	raw, ok := obj[key].([]any)
	require.True(t, ok, "expected %q to be a JSON array, got %T", key, obj[key])

	for _, v := range raw {
		if s, ok := v.(string); ok && s == target {
			return true
		}
	}

	return false
}

// TestIntegration_BlockUnblock_ReadPathExposure validates, end-to-end against a
// real Postgres/Mongo/Redis stack, that:
//
//  1. The /transactions/block and /transactions/unblock create endpoints persist
//     operations whose Type is BLOCK / UNBLOCK (Direction stays debit/credit).
//  2. Those types survive every read surface:
//     (a) single-transaction GET,
//     (b) transaction list,
//     (c) account operations list.
//  3. The derived Source/Destination arrays include the BLOCK/UNBLOCK legs
//     (debit alias -> Source, credit alias -> Destination) — the end-to-end
//     confirmation of the Task 1.3.3 derivation logic against a real DB.
func TestIntegration_BlockUnblock_ReadPathExposure(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		expectedType string
		sourceAlias  string
		destAlias    string
	}{
		{
			name:         "BLOCK transaction exposes Type=BLOCK across read paths",
			endpoint:     "/transactions/block",
			expectedType: cn.BLOCK,
			sourceAlias:  "@blk-source",
			destAlias:    "@blk-dest",
		},
		{
			name:         "UNBLOCK transaction exposes Type=UNBLOCK across read paths",
			endpoint:     "/transactions/unblock",
			expectedType: cn.UNBLOCK,
			sourceAlias:  "@unblk-source",
			destAlias:    "@unblk-dest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infra := setupBlockUnblockInfra(t)
			infra.seedLedgerSettings(t)

			// Arrange: funded source, empty destination.
			sourceAccountID := uuid.Must(libCommons.GenerateUUIDv7())
			destAccountID := uuid.Must(libCommons.GenerateUUIDv7())

			sourceParams := postgrestestutil.DefaultBalanceParams()
			sourceParams.Alias = tt.sourceAlias
			sourceParams.AssetCode = "USD"
			sourceParams.Available = decimal.NewFromInt(1000)
			sourceParams.OnHold = decimal.Zero
			postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
				infra.orgID, infra.ledgerID, sourceAccountID, sourceParams)

			destParams := postgrestestutil.DefaultBalanceParams()
			destParams.Alias = tt.destAlias
			destParams.AssetCode = "USD"
			destParams.Available = decimal.Zero
			destParams.OnHold = decimal.Zero
			postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
				infra.orgID, infra.ledgerID, destAccountID, destParams)

			// Act: create the typed transaction through the actual handler.
			txID := infra.createTransfer(t, tt.endpoint, tt.sourceAlias, tt.destAlias, "100")

			// Assert: persisted operations carry Type=BLOCK/UNBLOCK on both legs,
			// with the accounting Direction preserved (debit + credit).
			persisted := operationTypesByTransaction(t, infra, txID)
			require.Len(t, persisted, 2, "expected one debit and one credit operation")

			var sawDebit, sawCredit bool
			for _, op := range persisted {
				assert.Equal(t, tt.expectedType, op.Type,
					"persisted operation should carry Type=%s, got %s", tt.expectedType, op.Type)
				switch op.Direction {
				case cn.DirectionDebit:
					sawDebit = true
				case cn.DirectionCredit:
					sawCredit = true
				}
			}
			assert.True(t, sawDebit, "expected a debit-direction %s operation", tt.expectedType)
			assert.True(t, sawCredit, "expected a credit-direction %s operation", tt.expectedType)

			// (a) Single-transaction GET: operation types + derived Source/Destination.
			single := infra.getJSON(t, "/transactions/"+txID.String())
			assertSingleTransactionExposesType(t, single, tt)

			// (b) Transaction list: same transaction, same exposure.
			list := infra.getJSON(t, "/transactions")
			assertListExposesType(t, list, txID, tt)

			// (c) Account operations list: source account's leg carries the type.
			accountOps := infra.getJSON(t,
				"/accounts/"+sourceAccountID.String()+"/operations")
			assertAccountOperationsExposeType(t, accountOps, tt.expectedType)
		})
	}
}

func assertSingleTransactionExposesType(t *testing.T, single map[string]any, tt struct {
	name         string
	endpoint     string
	expectedType string
	sourceAlias  string
	destAlias    string
},
) {
	t.Helper()

	ops, ok := single["operations"].([]any)
	require.True(t, ok, "single transaction should carry an operations array")
	require.Len(t, ops, 2, "single transaction should carry two operations")

	for _, raw := range ops {
		op, ok := raw.(map[string]any)
		require.True(t, ok, "operation should decode as object")
		assert.Equal(t, tt.expectedType, op["type"],
			"single GET operation should expose Type=%s", tt.expectedType)
	}

	// Derived Source/Destination arrays must include the BLOCK/UNBLOCK legs.
	assert.True(t, arrayContains(t, single, "source", tt.sourceAlias),
		"single GET Source should include the debit-leg alias %s", tt.sourceAlias)
	assert.True(t, arrayContains(t, single, "destination", tt.destAlias),
		"single GET Destination should include the credit-leg alias %s", tt.destAlias)
}

func assertListExposesType(t *testing.T, list map[string]any, txID uuid.UUID, tt struct {
	name         string
	endpoint     string
	expectedType string
	sourceAlias  string
	destAlias    string
},
) {
	t.Helper()

	items, ok := list["items"].([]any)
	require.True(t, ok, "transaction list should carry an items array")

	var found map[string]any
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		require.True(t, ok, "list item should decode as object")
		if item["id"] == txID.String() {
			found = item
			break
		}
	}
	require.NotNil(t, found, "created transaction %s should appear in the list", txID)

	ops, ok := found["operations"].([]any)
	require.True(t, ok, "listed transaction should carry an operations array")
	require.Len(t, ops, 2, "listed transaction should carry two operations")

	for _, raw := range ops {
		op, ok := raw.(map[string]any)
		require.True(t, ok, "operation should decode as object")
		assert.Equal(t, tt.expectedType, op["type"],
			"list operation should expose Type=%s", tt.expectedType)
	}

	assert.True(t, arrayContains(t, found, "source", tt.sourceAlias),
		"list Source should include the debit-leg alias %s", tt.sourceAlias)
	assert.True(t, arrayContains(t, found, "destination", tt.destAlias),
		"list Destination should include the credit-leg alias %s", tt.destAlias)
}

func assertAccountOperationsExposeType(t *testing.T, accountOps map[string]any, expectedType string) {
	t.Helper()

	items, ok := accountOps["items"].([]any)
	require.True(t, ok, "account operations should carry an items array")
	require.NotEmpty(t, items, "account should have at least one operation")

	for _, raw := range items {
		op, ok := raw.(map[string]any)
		require.True(t, ok, "operation should decode as object")
		assert.Equal(t, expectedType, op["type"],
			"account operations list should expose Type=%s", expectedType)
	}
}
