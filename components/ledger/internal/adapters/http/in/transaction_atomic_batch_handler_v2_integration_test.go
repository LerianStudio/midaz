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
	"github.com/stretchr/testify/require"

	postgrescompletion "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// These tests intentionally share one real PostgreSQL/MongoDB/Valkey fixture.
// The ledgers and aliases are unique per scenario, preserving isolation while
// avoiding a container startup for every cardinality and failure case. They are
// sequential because the Huma test builders install process-global hooks.
type atomicBatchHTTPIntegrationFixture struct {
	infra  *testInfra
	app    *fiber.App
	engine command.Engine
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
	infra.handler.Command.AtomicTransactionBatchProjectionReader = infra.handler.Query
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

	return &atomicBatchHTTPIntegrationFixture{
		infra:  infra,
		app:    buildHumaV2DirectApp(t, infra.handler),
		engine: engine,
	}
}

func (fixture *atomicBatchHTTPIntegrationFixture) newLedger(t *testing.T) uuid.UUID {
	t.Helper()

	ledgerID := uuid.New()
	seedLedgerSettings(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID)

	return ledgerID
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
				require.NotEqual(t, uuid.Nil.String(), result.BatchID)
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

	t.Run("shared scope and request limits fail before execution", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		otherLedgerID := fixture.newLedger(t)
		scopeMismatch := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "first scope", "@scope-a", "@scope-b", 1),
			atomicBatchTransfer(fixture.infra.orgID, otherLedgerID, "second scope", "@scope-c", "@scope-d", 1),
		}
		response := postAtomicBatch(t, fixture.app, scopeMismatch, "scope-mismatch")
		body := drainBody(t, response)
		require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode, "body: %s", string(body))
		requireProblemCode(t, body, constant.ErrTransactionScopeMismatch.Error())

		fixture.infra.handler.TransactionBatchMaxSize = 1
		response = postAtomicBatch(t, fixture.app, scopeMismatch, "configured-cardinality")
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
		require.Zero(t, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerID))
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
