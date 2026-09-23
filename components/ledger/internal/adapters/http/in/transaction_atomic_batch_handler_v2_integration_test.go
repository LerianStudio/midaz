// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	postgrescompletion "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// These tests intentionally share one real PostgreSQL/MongoDB/Valkey fixture.
// The ledgers and aliases are unique per scenario, preserving isolation while
// avoiding a container startup for every cardinality and failure case. They are
// sequential because the Huma test builders install process-global hooks.
type atomicBatchHTTPIntegrationFixture struct {
	infra   *testInfra
	app     *fiber.App
	engine  command.Engine
	emitter *pkgStreaming.MockEmitter
}

type atomicBatchHTTPEvidenceResolver struct {
	repository redistransaction.EngineWriteBehindRepository
}

func (resolver atomicBatchHTTPEvidenceResolver) ResolveTransactionEvidence(
	ctx context.Context,
	reference command.TransactionEvidenceReference,
) (*command.TransactionWriteBehindEnvelope, error) {
	raw, _, err := resolver.repository.GetEngineTransactionEvidence(
		ctx,
		reference.OrganizationID,
		reference.LedgerID,
		reference.TransactionID,
		reference.ExecutionID,
	)
	if err != nil {
		return nil, err
	}

	return command.DecodeTransactionWriteBehindEnvelope(raw)
}

func setupAtomicBatchHTTPIntegrationFixture(t *testing.T) *atomicBatchHTTPIntegrationFixture {
	t.Helper()

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	infra := setupTestInfra(t)
	redisConn := redistestutil.CreateConnection(t, infra.redisContainer.Addr)
	engine, err := redisengine.NewAdapter(redisConn)
	require.NoError(t, err)

	completionStore := postgrescompletion.NewStore(
		infra.handler.Command.TransactionRepo,
		infra.handler.Command.OperationRepo,
	)
	batchRepository, ok := infra.redisRepo.(command.AtomicTransactionBatchIdempotencyRepository)
	require.True(t, ok, "Redis repository must expose the atomic batch state machine")

	var uuidSequence uint64
	var clockSequence int64
	fixedClock := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

	infra.handler.Command.AtomicTransactionBatchIdempotencyRepo = batchRepository
	infra.handler.Command.TransactionGroupRepo = transactiongroup.NewTransactionGroupPostgreSQLRepository(infra.pgConn)
	infra.handler.Command.AtomicTransactionBatchProjectionReader = infra.handler.Query
	infra.handler.Query.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}
	redisRepository, ok := infra.redisRepo.(*redistransaction.RedisConsumerRepository)
	require.True(t, ok, "Redis repository must support protected engine recovery acknowledgment")
	infra.handler.Command.TransactionEvidenceResolver = atomicBatchHTTPEvidenceResolver{repository: redisRepository}
	infra.handler.Command.EngineRecoveryAcknowledger = &atomicBatchHTTPRecoveryAcknowledger{
		repository:  redisRepository,
		completedAt: fixedClock,
	}
	infra.handler.Command.UUIDv7Generator = func() (uuid.UUID, error) {
		sequence := atomic.AddUint64(&uuidSequence, 1)

		return uuid.Parse(fmt.Sprintf("018f0c00-0000-7000-8000-%012x", sequence))
	}
	infra.handler.Command.Clock = func() time.Time {
		sequence := atomic.AddInt64(&clockSequence, 1)

		return fixedClock.Add(time.Duration(sequence) * time.Microsecond)
	}
	infra.handler.Command.Engine = engine
	infra.handler.Command.AppliedTransactionCompleter = command.NewTransactionCompletionService(
		completionStore,
		infra.metadataRepo,
	)
	infra.handler.TransactionBatchMaxSize = 50
	emitter := pkgStreaming.NewMockEmitter()
	infra.handler.Command.Streaming = emitter

	return &atomicBatchHTTPIntegrationFixture{
		infra:   infra,
		app:     buildHumaV2DirectApp(t, infra.handler),
		engine:  engine,
		emitter: emitter,
	}
}

// groupEvents returns the transaction_group facts published for one group, in
// emission order. The coordinator publishes them after the response is built,
// so callers wait for the count they expect.
func (fixture *atomicBatchHTTPIntegrationFixture) groupEvents(groupID string) []publishedGroupEvent {
	result := make([]publishedGroupEvent, 0)

	for _, emitted := range fixture.emitter.Events() {
		if !strings.HasPrefix(emitted.DefinitionKey, "transaction_group.") || emitted.Subject != groupID {
			continue
		}

		var payload events.TransactionGroupPayload
		if err := json.Unmarshal(emitted.Payload, &payload); err != nil {
			continue
		}

		result = append(result, publishedGroupEvent{key: emitted.DefinitionKey, payload: payload})
	}

	return result
}

type publishedGroupEvent struct {
	key     string
	payload events.TransactionGroupPayload
}

func (fixture *atomicBatchHTTPIntegrationFixture) requireGroupEvent(
	t *testing.T,
	groupID, key string,
) events.TransactionGroupPayload {
	t.Helper()

	require.Eventually(t, func() bool { return len(fixture.groupEvents(groupID)) > 0 }, 5*time.Second, 10*time.Millisecond,
		"group %s must publish %s", groupID, key)

	published := fixture.groupEvents(groupID)
	require.Len(t, published, 1, "one group operation publishes exactly one group fact")
	require.Equal(t, key, published[0].key)

	return published[0].payload
}

func (fixture *atomicBatchHTTPIntegrationFixture) newLedger(t *testing.T) uuid.UUID {
	t.Helper()

	ledgerID := uuid.New()
	seedLedgerSettings(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID)

	return ledgerID
}

func (fixture *atomicBatchHTTPIntegrationFixture) setCrossLedgerEnabled(t *testing.T, ledgerID uuid.UUID, enabled bool) {
	t.Helper()

	settings := fmt.Sprintf(`{"crossLedger":{"enabled":%t}}`, enabled)
	_, err := fixture.infra.pgContainer.DB.Exec(
		`UPDATE ledger SET settings = $1::jsonb WHERE organization_id = $2 AND id = $3`,
		settings,
		fixture.infra.orgID,
		ledgerID,
	)
	require.NoError(t, err)
	require.NoError(t, fixture.infra.redisRepo.Del(
		context.Background(),
		utils.LedgerSettingsInternalKey(fixture.infra.orgID, ledgerID),
	))
}

func (fixture *atomicBatchHTTPIntegrationFixture) setCrossLedgerRoutePolicy(t *testing.T, ledgerID uuid.UUID, enabled, validateRoutes bool) {
	t.Helper()

	settings := fmt.Sprintf(`{"crossLedger":{"enabled":%t},"accounting":{"validateRoutes":%t}}`, enabled, validateRoutes)
	_, err := fixture.infra.pgContainer.DB.Exec(
		`UPDATE ledger SET settings = $1::jsonb WHERE organization_id = $2 AND id = $3`,
		settings,
		fixture.infra.orgID,
		ledgerID,
	)
	require.NoError(t, err)
	require.NoError(t, fixture.infra.redisRepo.Del(
		context.Background(),
		utils.LedgerSettingsInternalKey(fixture.infra.orgID, ledgerID),
	))
}

func atomicBatchTransfer(
	organizationID, ledgerID uuid.UUID,
	description, debitAlias, creditAlias string,
	amount int64,
) CreateTransactionV2Request {
	amountText := strconv.FormatInt(amount, 10)
	leg := func(alias string) TransactionV2LegRequest {
		return TransactionV2LegRequest{
			Alias:          alias,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Amount:         amountText,
		}
	}

	return CreateTransactionV2Request{
		Description: description,
		Asset:       "USD",
		Amount:      amountText,
		Debits:      []TransactionV2LegRequest{leg(debitAlias)},
		Credits:     []TransactionV2LegRequest{leg(creditAlias)},
	}
}

func marshalAtomicBatchHTTPBody(t *testing.T, transactions []CreateTransactionV2Request) string {
	t.Helper()

	items := make([]CreateAtomicTransactionBatchV2ItemRequest, len(transactions))
	for index, transaction := range transactions {
		items[index] = CreateAtomicTransactionBatchV2ItemRequest{
			Action:                     "direct",
			Order:                      index + 1,
			CreateTransactionV2Request: transaction,
		}
	}

	body, err := json.Marshal(CreateAtomicTransactionBatchV2Request{Transactions: items})
	require.NoError(t, err)

	return string(body)
}

func postAtomicBatch(
	t *testing.T,
	app *fiber.App,
	transactions []CreateTransactionV2Request,
	idempotencyKey string,
) *http.Response {
	t.Helper()

	return postTransaction(
		t,
		app,
		v2CreateURL("batch"),
		marshalAtomicBatchHTTPBody(t, transactions),
		idempotencyKey,
	)
}

func decodeAtomicBatchResponse(
	t *testing.T,
	response *http.Response,
	wantStatus int,
) CreateAtomicTransactionBatchV2Response {
	t.Helper()

	body := drainBody(t, response)
	require.Equal(t, wantStatus, response.StatusCode, "unexpected HTTP status; body: %s", string(body))
	require.NotContains(t, string(body), `"batchId"`, "the public response must not expose the internal batch identifier")

	var result CreateAtomicTransactionBatchV2Response
	require.NoError(t, json.Unmarshal(body, &result), "response should be valid JSON; body: %s", string(body))

	return result
}

func requireCachedAvailable(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	ledgerID uuid.UUID,
	alias string,
	want int64,
) {
	t.Helper()

	balance := getBalanceFromRedis(
		t,
		context.Background(),
		fixture.infra.redisRepo,
		fixture.infra.orgID,
		ledgerID,
		alias,
		"default",
	)
	require.NotNil(t, balance, "balance %s must be materialized in the live cache", alias)
	require.True(t, decimal.NewFromInt(want).Equal(balance.Available),
		"balance %s: expected %d available, got %s", alias, want, balance.Available.String())
}

func atomicBatchAccountIDForBalance(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	balanceID uuid.UUID,
) uuid.UUID {
	t.Helper()

	var accountID uuid.UUID
	require.NoError(t, fixture.infra.pgContainer.DB.QueryRow(
		`SELECT account_id FROM balance WHERE id = $1`,
		balanceID,
	).Scan(&accountID))

	return accountID
}

func requireCachedOnHold(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	ledgerID uuid.UUID,
	alias string,
	want int64,
) {
	t.Helper()

	balance := getBalanceFromRedis(
		t,
		context.Background(),
		fixture.infra.redisRepo,
		fixture.infra.orgID,
		ledgerID,
		alias,
		"default",
	)
	require.NotNil(t, balance, "balance %s must be materialized in the live cache", alias)
	require.True(t, decimal.NewFromInt(want).Equal(balance.OnHold),
		"balance %s: expected %d on hold, got %s", alias, want, balance.OnHold.String())
}

func TestIntegration_AtomicTransactionBatchV2_EndToEndContract(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)

	t.Run("cardinality response order and repeated accounts", func(t *testing.T) {
		for _, size := range []int{1, 10, 50} {
			t.Run(strconv.Itoa(size), func(t *testing.T) {
				ledgerID := fixture.newLedger(t)
				sourceAlias := fmt.Sprintf("@cardinality-source-%d", size)
				destinationAlias := fmt.Sprintf("@cardinality-destination-%d", size)
				seedTransfer(
					t,
					fixture.infra.pgContainer.DB,
					fixture.infra.orgID,
					ledgerID,
					sourceAlias,
					destinationAlias,
					int64(size*10),
				)

				transactions := make([]CreateTransactionV2Request, size)
				for index := range transactions {
					transactions[index] = atomicBatchTransfer(
						fixture.infra.orgID,
						ledgerID,
						fmt.Sprintf("ordered-%02d-of-%02d", index, size),
						sourceAlias,
						destinationAlias,
						10,
					)
				}

				result := decodeAtomicBatchResponse(
					t,
					postAtomicBatch(t, fixture.app, transactions, fmt.Sprintf("cardinality-%d", size)),
					http.StatusCreated,
				)
				require.Len(t, result.Transactions, size)
				for index := range result.Transactions {
					require.Equal(t, transactions[index].Description, result.Transactions[index].Description)
				}
				require.Equal(t, size, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
				requireCachedAvailable(t, fixture, ledgerID, sourceAlias, 0)
				requireCachedAvailable(t, fixture, ledgerID, destinationAlias, int64(size*10))
			})
		}
	})

	t.Run("earlier credit funds later debit", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@ordered-a", "@ordered-b", 100)
		_, finalBalanceID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@ordered-unused", "@ordered-c", 0)

		transactions := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "credit b", "@ordered-a", "@ordered-b", 100),
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "debit b", "@ordered-b", "@ordered-c", 100),
		}
		result := decodeAtomicBatchResponse(
			t,
			postAtomicBatch(t, fixture.app, transactions, "ordered-credit-before-debit"),
			http.StatusCreated,
		)

		require.Equal(t, "credit b", result.Transactions[0].Description)
		require.Equal(t, "debit b", result.Transactions[1].Description)
		requireCachedAvailable(t, fixture, ledgerID, "@ordered-a", 0)
		requireCachedAvailable(t, fixture, ledgerID, "@ordered-b", 0)
		requireCachedAvailable(t, fixture, ledgerID, "@ordered-c", 100)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, finalBalanceID))
	})

	t.Run("named balance is preserved by an atomic batch item", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		defaultID, destinationID := seedTransfer(
			t,
			fixture.infra.pgContainer.DB,
			fixture.infra.orgID,
			ledgerID,
			"@batch-named-source",
			"@batch-named-destination",
			1000,
		)
		foodID := seedAdditionalBalanceForV2(
			t,
			fixture.infra.pgContainer.DB,
			fixture.infra.orgID,
			ledgerID,
			"@batch-named-source",
			"food",
			500,
		)

		transaction := atomicBatchTransfer(
			fixture.infra.orgID,
			ledgerID,
			"named balance batch",
			"@batch-named-source",
			"@batch-named-destination",
			100,
		)
		transaction.Debits[0].BalanceKey = "food"

		result := decodeAtomicBatchResponse(
			t,
			postAtomicBatch(t, fixture.app, []CreateTransactionV2Request{transaction}, "named-balance-batch"),
			http.StatusCreated,
		)
		require.Len(t, result.Transactions, 1)
		require.Len(t, result.Transactions[0].Operations, 2)
		for _, operation := range result.Transactions[0].Operations {
			if operation.AccountAlias == "@batch-named-source" {
				assert.Equal(t, "food", operation.BalanceKey)
			}
		}

		requireCachedBalanceAvailable(t, context.Background(), fixture.infra, ledgerID, "@batch-named-source", "food", 400)
		require.Nil(t, getBalanceFromRedis(
			t,
			context.Background(),
			fixture.infra.redisRepo,
			fixture.infra.orgID,
			ledgerID,
			"@batch-named-source",
			constant.DefaultBalanceKey,
		), "the untouched default balance must not be materialized in Redis")
		requireCachedBalanceAvailable(t, context.Background(), fixture.infra, ledgerID, "@batch-named-destination", constant.DefaultBalanceKey, 100)
		requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, defaultID))
		requireDecimalEqual(t, decimal.NewFromInt(500), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, foodID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationID))
	})

	t.Run("closed and closing accounts refuse the whole batch", func(t *testing.T) {
		tests := []struct {
			name       string
			wantCode   string
			wantStatus int
			protect    func(*testing.T, uuid.UUID, uuid.UUID)
		}{
			{
				name:       "closed account",
				wantCode:   constant.ErrAccountClosed.Error(),
				wantStatus: http.StatusUnprocessableEntity,
				protect: func(t *testing.T, ledgerID, accountID uuid.UUID) {
					closedAt := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
					require.NoError(t, fixture.infra.redisRepo.SetAccountClosedMarker(
						context.Background(), fixture.infra.orgID, ledgerID, accountID, closedAt,
					))
				},
			},
			{
				name:       "closing account",
				wantCode:   constant.ErrAccountClosingInProgress.Error(),
				wantStatus: http.StatusConflict,
				protect: func(t *testing.T, ledgerID, accountID uuid.UUID) {
					token := uuid.NewString()
					acquired, err := fixture.infra.redisRepo.AcquireAccountClosingMarker(
						context.Background(), fixture.infra.orgID, ledgerID, accountID, token,
					)
					require.NoError(t, err)
					require.True(t, acquired)
					t.Cleanup(func() {
						released, releaseErr := fixture.infra.redisRepo.ReleaseAccountClosingMarker(
							context.Background(), fixture.infra.orgID, ledgerID, accountID, token,
						)
						require.NoError(t, releaseErr)
						require.True(t, released)
					})
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				ledgerID := fixture.newLedger(t)
				sourceID, destinationID := seedTransfer(
					t,
					fixture.infra.pgContainer.DB,
					fixture.infra.orgID,
					ledgerID,
					"@protected-source-"+strings.ReplaceAll(test.name, " ", "-"),
					"@protected-destination-"+strings.ReplaceAll(test.name, " ", "-"),
					100,
				)
				destinationAccountID := atomicBatchAccountIDForBalance(t, fixture, destinationID)
				test.protect(t, ledgerID, destinationAccountID)

				transaction := atomicBatchTransfer(
					fixture.infra.orgID,
					ledgerID,
					"protected account refusal",
					"@protected-source-"+strings.ReplaceAll(test.name, " ", "-"),
					"@protected-destination-"+strings.ReplaceAll(test.name, " ", "-"),
					100,
				)
				response := postAtomicBatch(
					t,
					fixture.app,
					[]CreateTransactionV2Request{transaction},
					"protected-"+strings.ReplaceAll(test.name, " ", "-"),
				)
				body := drainBody(t, response)

				require.Equal(t, test.wantStatus, response.StatusCode, "body: %s", string(body))
				requireProblemCode(t, body, test.wantCode)
				require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
				requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, sourceID))
				requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationID))
			})
		}
	})

	t.Run("reversed order refuses without mutation", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		sourceID, middleID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@reversed-a", "@reversed-b", 100)
		_, destinationID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@reversed-unused", "@reversed-c", 0)

		transactions := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "debit b first", "@reversed-b", "@reversed-c", 100),
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "credit b later", "@reversed-a", "@reversed-b", 100),
		}
		response := postAtomicBatch(t, fixture.app, transactions, "reversed-order")
		body := drainBody(t, response)

		require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrInsufficientFunds.Error())
		require.Contains(t, string(body), "body.transactions[0]")
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, sourceID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, middleID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationID))
		require.Nil(t, getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, "@reversed-a", "default"))
		require.Nil(t, getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, "@reversed-b", "default"))
		require.Nil(t, getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, "@reversed-c", "default"))
	})

	t.Run("cross-ledger direct batch and request limits", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		otherLedgerID := fixture.newLedger(t)
		fixture.setCrossLedgerEnabled(t, ledgerID, true)
		fixture.setCrossLedgerEnabled(t, otherLedgerID, true)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@scope-a", "@scope-b", 1)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, otherLedgerID, "@scope-c", "@scope-d", 1)
		crossLedger := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "first scope", "@scope-a", "@scope-b", 1),
			atomicBatchTransfer(fixture.infra.orgID, otherLedgerID, "second scope", "@scope-c", "@scope-d", 1),
		}
		result := decodeAtomicBatchResponse(t, postAtomicBatch(t, fixture.app, crossLedger, "cross-ledger-batch"), http.StatusCreated)
		require.Len(t, result.Transactions, 2)
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, otherLedgerID))
		requireCachedAvailable(t, fixture, ledgerID, "@scope-b", 1)
		requireCachedAvailable(t, fixture, otherLedgerID, "@scope-d", 1)

		disabledLedgerID := fixture.newLedger(t)
		disabled := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "enabled scope", "@scope-a", "@scope-b", 1),
			atomicBatchTransfer(fixture.infra.orgID, disabledLedgerID, "disabled scope", "@disabled-a", "@disabled-b", 1),
		}
		response := postAtomicBatch(t, fixture.app, disabled, "cross-ledger-disabled")
		body := drainBody(t, response)
		require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrCrossLedgerNotEnabled.Error())

		fixture.infra.handler.TransactionBatchMaxSize = 1
		response = postAtomicBatch(t, fixture.app, crossLedger, "configured-cardinality")
		body = drainBody(t, response)
		require.Equal(t, http.StatusBadRequest, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrTransactionBatchCardinality.Error())
		fixture.infra.handler.TransactionBatchMaxSize = 50

		manyLegs := make([]CreateTransactionV2Request, 2)
		for transactionIndex := range manyLegs {
			manyLegs[transactionIndex] = CreateTransactionV2Request{
				Description: fmt.Sprintf("many legs %d", transactionIndex),
				Asset:       "USD",
				Amount:      "500",
				Debits: []TransactionV2LegRequest{{
					Alias:          fmt.Sprintf("@many-source-%d", transactionIndex),
					OrganizationID: fixture.infra.orgID.String(),
					LedgerID:       ledgerID.String(),
					Amount:         "500",
				}},
				Credits: make([]TransactionV2LegRequest, 500),
			}
			for legIndex := range manyLegs[transactionIndex].Credits {
				manyLegs[transactionIndex].Credits[legIndex] = TransactionV2LegRequest{
					Alias:          fmt.Sprintf("@many-destination-%d-%d", transactionIndex, legIndex),
					OrganizationID: fixture.infra.orgID.String(),
					LedgerID:       ledgerID.String(),
					Amount:         "1",
				}
			}
		}
		response = postAtomicBatch(t, fixture.app, manyLegs, "aggregate-leg-limit")
		body = drainBody(t, response)
		require.Equal(t, http.StatusBadRequest, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrTransactionBatchInputLegsLimitExceeded.Error())
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
	})

	t.Run("middle item refusal rolls back the whole batch", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		sourceID, destinationID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@rollback-a", "@rollback-b", 100)
		_, emptyID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@rollback-unused", "@rollback-c", 0)

		transactions := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "would succeed", "@rollback-a", "@rollback-b", 50),
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "must fail", "@rollback-c", "@rollback-a", 1),
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "must not run", "@rollback-a", "@rollback-b", 25),
		}
		response := postAtomicBatch(t, fixture.app, transactions, "middle-item-rollback")
		body := drainBody(t, response)

		require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrInsufficientFunds.Error())
		require.Contains(t, string(body), "body.transactions[1]")
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
		requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, sourceID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationID))
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, emptyID))
		require.Nil(t, getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, "@rollback-a", "default"))
		require.Nil(t, getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, "@rollback-b", "default"))
		require.Nil(t, getBalanceFromRedis(t, context.Background(), fixture.infra.redisRepo, fixture.infra.orgID, ledgerID, "@rollback-c", "default"))
	})

	t.Run("concurrent observer sees no prefix state", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@observe-a", "@observe-b", 100)
		_, _ = seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@observe-unused", "@observe-c", 0)

		observer := &postExecutionBlockingEngine{
			delegate: fixture.engine,
			applied:  make(chan struct{}),
			release:  make(chan struct{}),
		}
		fixture.infra.handler.Command.Engine = observer
		defer func() { fixture.infra.handler.Command.Engine = fixture.engine }()

		transactions := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "observe first", "@observe-a", "@observe-b", 60),
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "observe second", "@observe-b", "@observe-c", 40),
		}
		request := httptest.NewRequest(
			http.MethodPost,
			v2CreateURL("batch"),
			strings.NewReader(marshalAtomicBatchHTTPBody(t, transactions)),
		)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Idempotency", "concurrent-observation")

		type responseResult struct {
			response *http.Response
			err      error
		}
		resultChannel := make(chan responseResult, 1)
		go func() {
			response, err := fixture.app.Test(request, fiber.TestConfig{Timeout: 0})
			resultChannel <- responseResult{response: response, err: err}
		}()

		select {
		case <-observer.applied:
		case <-time.After(10 * time.Second):
			t.Fatal("accounting execution did not reach the observation barrier")
		}

		// The HTTP request is still blocked before projection completion. A
		// concurrent balance observer can only see the complete Lua publication,
		// never the prefix after transaction zero and before transaction one.
		requireCachedAvailable(t, fixture, ledgerID, "@observe-a", 40)
		requireCachedAvailable(t, fixture, ledgerID, "@observe-b", 20)
		requireCachedAvailable(t, fixture, ledgerID, "@observe-c", 40)

		close(observer.release)
		result := <-resultChannel
		require.NoError(t, result.err)
		decoded := decodeAtomicBatchResponse(t, result.response, http.StatusCreated)
		require.Len(t, decoded.Transactions, 2)
	})

	t.Run("existing v1 and singular v2 creates remain compatible", func(t *testing.T) {
		fixture.infra.handler.Command.Engine = fixture.engine

		ledgerV2 := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerV2, "@singular-v2-source", "@singular-v2-destination", 100)
		singularV2 := atomicBatchTransfer(
			fixture.infra.orgID,
			ledgerV2,
			"unchanged singular v2",
			"@singular-v2-source",
			"@singular-v2-destination",
			100,
		)
		singularBody, err := json.Marshal(singularV2)
		require.NoError(t, err)
		response := postTransaction(t, fixture.app, v2CreateURL("direct"), string(singularBody), "unchanged-v2")
		_ = decodeTxResponse(t, response, http.StatusCreated)
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerV2))

		ledgerV1 := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerV1, "@src", "@dst", 100)
		v1App := buildHumaTransactionApp(t, fixture.infra.handler, true)
		response = postTransaction(t, v1App, v1JSONURL(fixture.infra.orgID, ledgerV1), equivalentV1Body, "unchanged-v1")
		_ = decodeTxResponse(t, response, http.StatusCreated)
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerV1))
	})
}

func TestIntegration_DirectV2CrossLedger_OneAtomicGroup(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	ledgerA := fixture.newLedger(t)
	ledgerB := fixture.newLedger(t)
	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)

	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, "@cross-source", "@external/USD", 100)
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", "@cross-destination", 100)

	request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "cross-ledger direct", "@cross-source", "@cross-destination", 100)
	request.Credits[0].LedgerID = ledgerB.String()
	raw, err := json.Marshal(request)
	require.NoError(t, err)

	response := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "cross-ledger-direct")
	body := drainBody(t, response)
	require.Equal(t, http.StatusCreated, response.StatusCode, "body: %s", string(body))

	var result CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(body, &result))
	require.NotNil(t, result.GroupID)
	require.Len(t, result.Transactions, 2)
	require.Equal(t, *result.GroupID, *result.Transactions[0].GroupID)
	require.Equal(t, *result.GroupID, *result.Transactions[1].GroupID)
	require.Equal(t, ledgerA.String(), result.Transactions[0].LedgerID)
	require.Equal(t, ledgerB.String(), result.Transactions[1].LedgerID)
	require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
	require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
	requireCachedAvailable(t, fixture, ledgerA, "@cross-source", 0)
	requireCachedAvailable(t, fixture, ledgerB, "@cross-destination", 100)

	posted := fixture.requireGroupEvent(t, *result.GroupID, events.TransactionGroupPostedDefinition.Key())
	require.Equal(t, constant.APPROVED, posted.Status)
	require.Equal(t, 2, posted.LedgerCount)
	require.Len(t, posted.Parts, 2)
	require.Equal(t, result.Transactions[0].ID, posted.Parts[0].TransactionID)
	require.Equal(t, events.TransactionGroupRoleOrigin, posted.Parts[0].Role)
	require.Equal(t, result.Transactions[1].ID, posted.Parts[1].TransactionID)
	require.Equal(t, events.TransactionGroupRoleDestination, posted.Parts[1].Role)

	replay := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "cross-ledger-direct")
	replayBody := drainBody(t, replay)
	require.Equal(t, http.StatusCreated, replay.StatusCode, "body: %s", string(replayBody))
	require.Equal(t, "true", replay.Header.Get("X-Idempotency-Replayed"))
	require.JSONEq(t, string(body), string(replayBody))
	require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
	require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
	require.Never(t, func() bool { return len(fixture.groupEvents(*result.GroupID)) > 1 }, 300*time.Millisecond, 10*time.Millisecond,
		"a replay republishes nothing")

	t.Run("disabled participant is rejected", func(t *testing.T) {
		enabledLedger := fixture.newLedger(t)
		disabledLedger := fixture.newLedger(t)
		fixture.setCrossLedgerEnabled(t, enabledLedger, true)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, enabledLedger, "@disabled-source", "@external/USD", 10)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, disabledLedger, "@external/USD", "@disabled-destination", 10)
		request := atomicBatchTransfer(fixture.infra.orgID, enabledLedger, "disabled participant", "@disabled-source", "@disabled-destination", 10)
		request.Credits[0].LedgerID = disabledLedger.String()
		raw, err := json.Marshal(request)
		require.NoError(t, err)

		response := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "cross-ledger-disabled-direct")
		body := drainBody(t, response)
		require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrCrossLedgerNotEnabled.Error())
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, enabledLedger))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, disabledLedger))
	})

	t.Run("route-validating participant is rejected", func(t *testing.T) {
		sourceLedger := fixture.newLedger(t)
		destinationLedger := fixture.newLedger(t)
		fixture.setCrossLedgerRoutePolicy(t, sourceLedger, true, false)
		fixture.setCrossLedgerRoutePolicy(t, destinationLedger, true, true)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, sourceLedger, "@route-source", "@external/USD", 10)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, destinationLedger, "@external/USD", "@route-destination", 10)
		request := atomicBatchTransfer(fixture.infra.orgID, sourceLedger, "route gate", "@route-source", "@route-destination", 10)
		request.Credits[0].LedgerID = destinationLedger.String()
		raw, err := json.Marshal(request)
		require.NoError(t, err)

		response := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "cross-ledger-route-direct")
		body := drainBody(t, response)
		require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrCrossLedgerRouteValidationUnsupported.Error())
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, sourceLedger))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, destinationLedger))
	})
}

func TestIntegration_RevertV2CrossLedger_RevertsTheWholeGroupAtomically(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	ledgerA := fixture.newLedger(t)
	ledgerB := fixture.newLedger(t)
	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)

	sourceBalanceID, _ := seedTransfer(
		t,
		fixture.infra.pgContainer.DB,
		fixture.infra.orgID,
		ledgerA,
		"@group-revert-source",
		"@external/USD",
		100,
	)
	_, destinationBalanceID := seedTransfer(
		t,
		fixture.infra.pgContainer.DB,
		fixture.infra.orgID,
		ledgerB,
		"@external/USD",
		"@group-revert-destination",
		100,
	)

	request := atomicBatchTransfer(
		fixture.infra.orgID,
		ledgerA,
		"cross-ledger group revert origin",
		"@group-revert-source",
		"@group-revert-destination",
		100,
	)
	request.Credits[0].LedgerID = ledgerB.String()
	raw, err := json.Marshal(request)
	require.NoError(t, err)

	originResponse := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "cross-ledger-group-revert-origin")
	originBody := drainBody(t, originResponse)
	require.Equal(t, http.StatusCreated, originResponse.StatusCode, "body: %s", string(originBody))

	var origin CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(originBody, &origin))
	require.NotNil(t, origin.GroupID)
	require.Len(t, origin.Transactions, 2)
	requireCachedAvailable(t, fixture, ledgerA, "@group-revert-source", 0)
	requireCachedAvailable(t, fixture, ledgerB, "@group-revert-destination", 100)

	countedEngine := &countingAtomicBatchEngine{delegate: fixture.engine}
	fixture.infra.handler.Command.Engine = countedEngine

	selectedOrigin := origin.Transactions[1]
	selectedOriginID := uuid.MustParse(selectedOrigin.ID)
	revertURL := v2RevertURL(fixture.infra.orgID, uuid.MustParse(selectedOrigin.LedgerID), selectedOriginID)
	revertResponse := postTransaction(t, fixture.app, revertURL, "", "cross-ledger-group-revert")
	revertBody := drainBody(t, revertResponse)
	require.Equal(t, http.StatusCreated, revertResponse.StatusCode, "body: %s", string(revertBody))
	require.Equal(t, "false", revertResponse.Header.Get("X-Idempotency-Replayed"))

	var reverted CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(revertBody, &reverted))
	require.NotNil(t, reverted.GroupID)
	require.NotNil(t, reverted.RevertedGroupID)
	require.Equal(t, *origin.GroupID, *reverted.RevertedGroupID)
	require.NotEqual(t, *origin.GroupID, *reverted.GroupID)
	require.Len(t, reverted.Transactions, 2)
	require.Equal(t, int64(1), countedEngine.calls.Load(), "the whole group must use one atomic engine execution")

	revertedFact := fixture.requireGroupEvent(t, *reverted.GroupID, events.TransactionGroupRevertedDefinition.Key())
	require.NotNil(t, revertedFact.RevertedGroupID)
	require.Equal(t, *origin.GroupID, *revertedFact.RevertedGroupID)
	require.Len(t, revertedFact.Parts, 2)
	require.Equal(t, events.TransactionGroupRoleOrigin, revertedFact.Parts[0].Role, "the reversal debits the former destination")
	require.Equal(t, events.TransactionGroupRoleDestination, revertedFact.Parts[1].Role)

	for index, reversal := range reverted.Transactions {
		originIndex := len(origin.Transactions) - 1 - index
		require.Equal(t, index+1, reversal.Order)
		require.NotNil(t, reversal.GroupID)
		require.Equal(t, *reverted.GroupID, *reversal.GroupID)
		require.NotNil(t, reversal.ParentTransactionID)
		require.Equal(t, origin.Transactions[originIndex].ID, *reversal.ParentTransactionID)
		require.Equal(t, origin.Transactions[originIndex].LedgerID, reversal.LedgerID)

		persistedParent := postgrestestutil.GetTransactionParentID(
			t,
			fixture.infra.pgContainer.DB,
			uuid.MustParse(reversal.ID),
		)
		require.NotNil(t, persistedParent)
		require.Equal(t, uuid.MustParse(origin.Transactions[originIndex].ID), *persistedParent)
	}

	requireCachedAvailable(t, fixture, ledgerA, "@group-revert-source", 100)
	requireCachedAvailable(t, fixture, ledgerB, "@group-revert-destination", 0)
	requireDecimalEqual(t, decimal.NewFromInt(100), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, sourceBalanceID))
	requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationBalanceID))
	require.Equal(t, 2, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
	require.Equal(t, 2, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))

	secondResponse := postTransaction(t, fixture.app, revertURL, "", "cross-ledger-group-revert")
	secondBody := drainBody(t, secondResponse)
	require.Equal(t, http.StatusConflict, secondResponse.StatusCode, "body: %s", string(secondBody))
	requireProblemCode(t, secondBody, constant.ErrTransactionIDHasAlreadyParentTransaction.Error())
	require.Equal(t, int64(1), countedEngine.calls.Load(), "a second revert must fail before accounting")
}

func TestIntegration_HoldCommitCancelV2CrossLedger_GroupLifecycle(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)

	t.Run("hold then commit creates destinations and remains revertible", func(t *testing.T) {
		ledgerA := fixture.newLedger(t)
		ledgerB := fixture.newLedger(t)
		fixture.setCrossLedgerEnabled(t, ledgerA, true)
		fixture.setCrossLedgerEnabled(t, ledgerB, true)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, "@held-source", "@external/USD", 100)
		_, destinationBalanceID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", "@committed-destination", 100)

		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "cross-ledger hold commit", "@held-source", "@committed-destination", 100)
		request.Credits[0].LedgerID = ledgerB.String()
		raw, err := json.Marshal(request)
		require.NoError(t, err)

		holdResponse := postTransaction(t, fixture.app, v2CreateURL("hold"), string(raw), "cross-ledger-hold-commit")
		holdBody := drainBody(t, holdResponse)
		require.Equal(t, http.StatusCreated, holdResponse.StatusCode, "body: %s", string(holdBody))
		var held CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(holdBody, &held))
		require.NotNil(t, held.GroupID)
		require.Len(t, held.Transactions, 1)
		require.Equal(t, constant.PENDING, held.Transactions[0].Status.Code)
		require.Equal(t, ledgerA.String(), held.Transactions[0].LedgerID)
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
		requireCachedAvailable(t, fixture, ledgerA, "@held-source", 0)
		requireCachedOnHold(t, fixture, ledgerA, "@held-source", 100)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationBalanceID))

		countedEngine := &countingAtomicBatchEngine{delegate: fixture.engine}
		fixture.infra.handler.Command.Engine = countedEngine
		originID := uuid.MustParse(held.Transactions[0].ID)
		commitURL := v2CommitURL(fixture.infra.orgID, ledgerA, originID)
		commitResponse := postTransaction(t, fixture.app, commitURL, "", "")
		commitBody := drainBody(t, commitResponse)
		require.Equal(t, http.StatusCreated, commitResponse.StatusCode, "body: %s", string(commitBody))
		var committed CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(commitBody, &committed))
		require.NotNil(t, committed.GroupID)
		require.Equal(t, *held.GroupID, *committed.GroupID)
		require.Len(t, committed.Transactions, 2)
		for _, tran := range committed.Transactions {
			require.Equal(t, constant.APPROVED, tran.Status.Code)
			require.Equal(t, *held.GroupID, *tran.GroupID)
		}
		require.Equal(t, int64(1), countedEngine.calls.Load())
		requireCachedAvailable(t, fixture, ledgerA, "@held-source", 0)
		requireCachedOnHold(t, fixture, ledgerA, "@held-source", 0)
		requireCachedAvailable(t, fixture, ledgerB, "@committed-destination", 100)
		require.Equal(t, constant.APPROVED, crossLedgerGroupStatus(t, fixture, *held.GroupID))

		committedFact := fixture.requireGroupEvent(t, *held.GroupID, events.TransactionGroupCommittedDefinition.Key())
		require.Equal(t, constant.APPROVED, committedFact.Status)
		require.Len(t, committedFact.Parts, 2, "a hold publishes nothing; the commit publishes every part")
		require.Equal(t, originID.String(), committedFact.Parts[0].TransactionID)
		require.Equal(t, events.TransactionGroupRoleOrigin, committedFact.Parts[0].Role)
		require.Equal(t, events.TransactionGroupRoleDestination, committedFact.Parts[1].Role)

		second := postTransaction(t, fixture.app, commitURL, "", "")
		secondBody := drainBody(t, second)
		require.Equal(t, http.StatusUnprocessableEntity, second.StatusCode, "body: %s", string(secondBody))
		requireProblemCode(t, secondBody, constant.ErrCrossLedgerGroupNotPending.Error())
		require.Equal(t, int64(1), countedEngine.calls.Load())

		selected := committed.Transactions[1]
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
		require.Equal(t, constant.APPROVED, postgrestestutil.GetTransactionStatus(t, fixture.infra.pgContainer.DB, uuid.MustParse(selected.ID)))
		revert := postTransaction(
			t,
			fixture.app,
			v2RevertURL(fixture.infra.orgID, uuid.MustParse(selected.LedgerID), uuid.MustParse(selected.ID)),
			"",
			"cross-ledger-held-group-revert",
		)
		revertBody := drainBody(t, revert)
		require.Equal(t, http.StatusCreated, revert.StatusCode, "body: %s", string(revertBody))
		requireCachedAvailable(t, fixture, ledgerA, "@held-source", 100)
		requireCachedAvailable(t, fixture, ledgerB, "@committed-destination", 0)
	})

	t.Run("hold then cancel releases origins without creating destinations", func(t *testing.T) {
		ledgerA := fixture.newLedger(t)
		ledgerB := fixture.newLedger(t)
		fixture.setCrossLedgerEnabled(t, ledgerA, true)
		fixture.setCrossLedgerEnabled(t, ledgerB, true)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, "@canceled-source", "@external/USD", 100)
		_, destinationBalanceID := seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", "@untouched-destination", 100)

		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "cross-ledger hold cancel", "@canceled-source", "@untouched-destination", 100)
		request.Credits[0].LedgerID = ledgerB.String()
		raw, err := json.Marshal(request)
		require.NoError(t, err)

		holdResponse := postTransaction(t, fixture.app, v2CreateURL("hold"), string(raw), "cross-ledger-hold-cancel")
		holdBody := drainBody(t, holdResponse)
		require.Equal(t, http.StatusCreated, holdResponse.StatusCode, "body: %s", string(holdBody))
		var held CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(holdBody, &held))
		require.Len(t, held.Transactions, 1)

		countedEngine := &countingAtomicBatchEngine{delegate: fixture.engine}
		fixture.infra.handler.Command.Engine = countedEngine
		originID := uuid.MustParse(held.Transactions[0].ID)
		cancelURL := v2CancelURL(fixture.infra.orgID, ledgerA, originID)
		cancelResponse := postTransaction(t, fixture.app, cancelURL, "", "")
		cancelBody := drainBody(t, cancelResponse)
		require.Equal(t, http.StatusCreated, cancelResponse.StatusCode, "body: %s", string(cancelBody))
		var canceled CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(cancelBody, &canceled))
		require.Equal(t, *held.GroupID, *canceled.GroupID)
		require.Len(t, canceled.Transactions, 1)
		require.Equal(t, constant.CANCELED, canceled.Transactions[0].Status.Code)
		require.Equal(t, int64(1), countedEngine.calls.Load())
		requireCachedAvailable(t, fixture, ledgerA, "@canceled-source", 100)
		requireCachedOnHold(t, fixture, ledgerA, "@canceled-source", 0)
		requireDecimalEqual(t, decimal.Zero, postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationBalanceID))
		require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
		require.Equal(t, constant.CANCELED, crossLedgerGroupStatus(t, fixture, *held.GroupID))

		canceledFact := fixture.requireGroupEvent(t, *held.GroupID, events.TransactionGroupCanceledDefinition.Key())
		require.Equal(t, constant.CANCELED, canceledFact.Status)
		require.Len(t, canceledFact.Parts, 1)
		require.Equal(t, originID.String(), canceledFact.Parts[0].TransactionID)
		require.Equal(t, constant.CANCELED, canceledFact.Parts[0].Status)
	})
}

func TestIntegration_TransactionGroupReconciler_AlignsFromMembersAndDropsOrphans(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	ledgerA := fixture.newLedger(t)
	ledgerB := fixture.newLedger(t)
	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, "@reconciled-source", "@external/USD", 100)
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", "@reconciled-destination", 100)

	request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "cross-ledger reconciled commit", "@reconciled-source", "@reconciled-destination", 100)
	request.Credits[0].LedgerID = ledgerB.String()
	raw, err := json.Marshal(request)
	require.NoError(t, err)

	holdResponse := postTransaction(t, fixture.app, v2CreateURL("hold"), string(raw), "cross-ledger-reconciled-hold")
	holdBody := drainBody(t, holdResponse)
	require.Equal(t, http.StatusCreated, holdResponse.StatusCode, "body: %s", string(holdBody))

	var held CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(holdBody, &held))
	require.NotNil(t, held.GroupID)

	commitURL := v2CommitURL(fixture.infra.orgID, ledgerA, uuid.MustParse(held.Transactions[0].ID))
	commitResponse := postTransaction(t, fixture.app, commitURL, "", "")
	commitBody := drainBody(t, commitResponse)
	require.Equal(t, http.StatusCreated, commitResponse.StatusCode, "body: %s", string(commitBody))
	fixture.requireGroupEvent(t, *held.GroupID, events.TransactionGroupCommittedDefinition.Key())

	// A commit that applied its movement but never reached the status update
	// leaves the row PENDING while every member is already APPROVED.
	_, err = fixture.infra.pgContainer.DB.Exec(`UPDATE transaction_group SET status = 'PENDING' WHERE id = $1`, *held.GroupID)
	require.NoError(t, err)

	orphanCreatedAt := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	orphan := &transactiongroup.TransactionGroup{
		ID:             uuid.MustParse("018f0c00-0000-7000-8000-0000000fffff"),
		OrganizationID: fixture.infra.orgID,
		LedgerID:       ledgerA,
		Status:         constant.PENDING,
		AssetCode:      "USD",
		Intent:         []byte(`{"formatVersion":1,"asset":"USD","parts":[]}`),
		CreatedAt:      orphanCreatedAt,
		UpdatedAt:      orphanCreatedAt,
	}
	require.NoError(t, fixture.infra.handler.Command.TransactionGroupRepo.Create(context.Background(), orphan))

	// The status transition stamps updated_at with the database clock rather than
	// the fixture's frozen one, so the reconciler's "now" is anchored on the data:
	// an hour past the latest member change is at rest by any minimum age.
	var latestMemberChange time.Time
	require.NoError(t, fixture.infra.pgContainer.DB.QueryRow(
		`SELECT max(updated_at) FROM transaction WHERE group_id = $1`, *held.GroupID,
	).Scan(&latestMemberChange))
	reconcileAt := latestMemberChange.Add(time.Hour)
	fixture.infra.handler.Command.Clock = func() time.Time { return reconcileAt }

	stats := fixture.infra.handler.Command.ReconcileTransactionGroups(context.Background())

	require.Equal(t, command.TransactionGroupReconciliationStats{Scanned: 2, Repaired: 1, Deleted: 1}, stats)
	require.Equal(t, constant.APPROVED, crossLedgerGroupStatus(t, fixture, *held.GroupID))

	var remaining int
	require.NoError(t, fixture.infra.pgContainer.DB.QueryRow(
		`SELECT count(*) FROM transaction_group WHERE id = $1`, orphan.ID,
	).Scan(&remaining))
	require.Zero(t, remaining, "a member-less intent older than the orphan age is deleted")

	require.Eventually(t, func() bool { return len(fixture.groupEvents(*held.GroupID)) == 2 }, 5*time.Second, 10*time.Millisecond,
		"the reconciler that moved the row publishes the fact the coordinator could not")
	repaired := fixture.groupEvents(*held.GroupID)[1]
	require.Equal(t, events.TransactionGroupCommittedDefinition.Key(), repaired.key)
	require.Len(t, repaired.payload.Parts, 2)
	require.Empty(t, fixture.groupEvents(orphan.ID.String()))

	second := fixture.infra.handler.Command.ReconcileTransactionGroups(context.Background())
	require.Equal(t, command.TransactionGroupReconciliationStats{}, second, "an aligned tenant has nothing left to read")
}

func crossLedgerGroupStatus(t *testing.T, fixture *atomicBatchHTTPIntegrationFixture, groupID string) string {
	t.Helper()

	var status string
	err := fixture.infra.pgContainer.DB.QueryRow(
		`SELECT status FROM transaction_group WHERE id = $1`,
		groupID,
	).Scan(&status)
	require.NoError(t, err)

	return status
}

type countingAtomicBatchEngine struct {
	delegate command.Engine
	calls    atomic.Int64
}

type atomicBatchHTTPRecoveryAcknowledger struct {
	repository  *redistransaction.RedisConsumerRepository
	completedAt time.Time
}

func (acknowledger *atomicBatchHTTPRecoveryAcknowledger) AcknowledgeEngineRecovery(
	ctx context.Context,
	record *command.TransactionCompletionRecord,
	completion command.TransactionCompletionResult,
) error {
	field := record.TransactionID.String() + ":" + record.ExecutionID.String()
	raw, err := acknowledger.repository.ReadRecoveryMessage(
		ctx,
		redistransaction.RecoveryQueueSourceEngineRecover,
		field,
	)
	if err != nil || raw == "" {
		return err
	}

	terminal := completion.Outcome.TransactionStatus == constant.APPROVED ||
		completion.Outcome.TransactionStatus == constant.CANCELED
	status, err := acknowledger.repository.CompareAndDeleteRecoveryWithProtectionFrom(
		ctx,
		redistransaction.RecoveryQueueSourceEngineRecover,
		record.OrganizationID,
		record.LedgerID,
		field,
		raw,
		terminal,
		acknowledger.completedAt,
	)
	if err != nil {
		return err
	}
	if status == redistransaction.RecoveryAckReplaced {
		return fmt.Errorf("engine recovery record changed before acknowledgment")
	}

	return nil
}

func (engine *countingAtomicBatchEngine) Execute(
	ctx context.Context,
	input command.EngineExecution,
) (*accounting.ExecutionResult, error) {
	engine.calls.Add(1)

	return engine.delegate.Execute(ctx, input)
}

func (engine *countingAtomicBatchEngine) EnsureTransactionGuard(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	token string,
) error {
	bootstrapper, ok := engine.delegate.(command.EngineGuardBootstrapper)
	if !ok {
		return fmt.Errorf("counted engine delegate does not support transaction guards")
	}

	return bootstrapper.EnsureTransactionGuard(ctx, organizationID, ledgerID, transactionID, token)
}

// postExecutionBlockingEngine exposes the instant immediately after the real
// atomic Lua execution returns and before completion starts. Holding the HTTP
// request there makes the live cache observable without weakening production
// atomicity or replacing the real engine with a simulation.
type postExecutionBlockingEngine struct {
	delegate command.Engine
	applied  chan struct{}
	release  chan struct{}
}

func (engine *postExecutionBlockingEngine) Execute(
	ctx context.Context,
	input command.EngineExecution,
) (*accounting.ExecutionResult, error) {
	result, err := engine.delegate.Execute(ctx, input)
	close(engine.applied)
	<-engine.release

	return result, err
}
