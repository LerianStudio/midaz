// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestIntegration_PendingTransactionCachePreservesOperationsBeforeAsyncConsumer(t *testing.T) {
	ctrl := gomock.NewController(t)
	infra := setupAsyncTestInfra(t)
	installPendingCacheRedisHarness(ctrl, infra)

	type transitionResult struct {
		transactionID uuid.UUID
		status        string
		cachedIDs     []string
	}

	tests := []struct {
		name   string
		action string
		status string
	}{
		{name: "commit", action: "commit", status: constant.APPROVED},
		{name: "cancel", action: "cancel", status: constant.CANCELED},
	}

	results := make([]transitionResult, 0, len(tests))

	// Queue both lifecycles before starting the consumer. This keeps PostgreSQL
	// empty while GET by id is forced through the write-behind cache.
	for index, tt := range tests {
		sourceAlias := fmt.Sprintf("@pending-cache-source-%d", index)
		destinationAlias := fmt.Sprintf("@pending-cache-destination-%d", index)

		seedPendingCacheBalance(t, infra, sourceAlias, decimal.NewFromInt(1000))
		seedPendingCacheBalance(t, infra, destinationAlias, decimal.Zero)

		requestBody := fmt.Sprintf(`{
			"description": "pending cache %s integration test",
			"pending": true,
			"send": {
				"asset": "USD",
				"value": "100",
				"source": {"from": [{
					"accountAlias": %q,
					"amount": {"asset": "USD", "value": "100"}
				}]},
				"distribute": {"to": [{
					"accountAlias": %q,
					"amount": {"asset": "USD", "value": "100"}
				}]}
			}
		}`, tt.action, sourceAlias, destinationAlias)

		created := postPendingCacheRequest(t, infra, "/v1/organizations/"+infra.orgID.String()+
			"/ledgers/"+infra.ledgerID.String()+"/transactions/json", requestBody)
		require.Equalf(t, nethttp.StatusCreated, created.status, "pending create failed: %s", created.body)

		var pending transaction.Transaction
		require.NoError(t, json.Unmarshal(created.body, &pending))
		transactionID := uuid.MustParse(pending.ID)
		priorIDs := pendingCacheOperationIDs(pending.Operations)
		require.NotEmpty(t, priorIDs, "the PENDING create must expose its hold operations")

		transitioned := postPendingCacheRequest(t, infra, "/v1/organizations/"+infra.orgID.String()+
			"/ledgers/"+infra.ledgerID.String()+"/transactions/"+transactionID.String()+"/"+tt.action, "")
		require.Equalf(t, nethttp.StatusCreated, transitioned.status, "%s failed: %s", tt.action, transitioned.body)

		var rowCount int
		require.NoError(t, infra.pgContainer.DB.QueryRow(
			`SELECT COUNT(*) FROM transaction WHERE id = $1`, transactionID,
		).Scan(&rowCount))
		require.Zero(t, rowCount, "the consumer must still be stopped during the cache assertion")

		var cached transaction.Transaction
		var cacheHit string
		require.Eventually(t, func() bool {
			got, hit, ok := getPendingCacheTransaction(infra, transactionID)
			if !ok {
				return false
			}

			cached = got
			cacheHit = hit

			return hit == "true" && got.Status.Code == tt.status && len(got.Operations) > len(pending.Operations)
		}, 5*time.Second, 25*time.Millisecond,
			"GET by id did not observe the terminal write-behind entry for %s", tt.action)

		cachedIDs := pendingCacheOperationIDs(cached.Operations)
		assert.Equal(t, "true", cacheHit)
		for _, priorID := range priorIDs {
			assert.Contains(t, cachedIDs, priorID,
				"the terminal cache entry must retain every operation created by the hold")
		}

		results = append(results, transitionResult{
			transactionID: transactionID,
			status:        tt.status,
			cachedIDs:     cachedIDs,
		})
	}

	consumer := newTestMultiQueueConsumer(infra.consumerRoutes, infra.commandUC)
	consumerStarted := make(chan struct{})

	go func() {
		close(consumerStarted)
		_ = consumer.run()
	}()

	<-consumerStarted

	for _, result := range results {
		require.Eventually(t, func() bool {
			var status string
			if err := infra.pgContainer.DB.QueryRow(
				`SELECT status FROM transaction WHERE id = $1`, result.transactionID,
			).Scan(&status); err != nil {
				return false
			}

			return status == result.status
		}, 15*time.Second, 100*time.Millisecond,
			"the async consumer did not persist transaction %s as %s", result.transactionID, result.status)

		var persistedIDs []string
		require.Eventually(t, func() bool {
			var err error
			persistedIDs, err = pendingCachePersistedOperationIDs(infra.pgContainer.DB, result.transactionID)

			return err == nil && len(persistedIDs) == len(result.cachedIDs)
		}, 15*time.Second, 100*time.Millisecond,
			"the async consumer did not persist the complete operation set for %s", result.transactionID)

		assert.ElementsMatch(t, result.cachedIDs, persistedIDs,
			"the pre-consumer cached operation set must equal PostgreSQL after consumption")
	}
}

// installPendingCacheRedisHarness leaves the state under test on real Redis:
// write-behind blobs and backup entries delegate to the container-backed
// repository. Only the atomic balance mutation is replaced with deterministic
// snapshots because this branch's unrelated dual-projection Lua rejects the
// transaction harness's indexed aliases before any write. RabbitMQ publication
// and consumption and PostgreSQL persistence remain production implementations.
func installPendingCacheRedisHarness(ctrl *gomock.Controller, infra *testAsyncInfra) {
	realRedis := infra.redisRepo
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	redisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, value []byte, ttl time.Duration) error {
			return realRedis.SetBytes(ctx, key, value, ttl)
		}).AnyTimes()
	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string) ([]byte, error) {
			return realRedis.GetBytes(ctx, key)
		}).AnyTimes()
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string) error {
			return realRedis.Del(ctx, key)
		}).AnyTimes()
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string, value []byte) error {
			return realRedis.AddMessageToQueue(ctx, key, value)
		}).AnyTimes()
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string) ([]byte, error) {
			return realRedis.ReadMessageFromQueue(ctx, key)
		}).AnyTimes()
	redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key string) error {
			return realRedis.RemoveMessageFromQueue(ctx, key)
		}).AnyTimes()
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, status string, _ bool,
		operations []mmodel.BalanceOperation, _ *mtransaction.AccountBlockExceptionBinding,
	) (*mmodel.BalanceAtomicResult, error) {
		return pendingCacheBalanceResult(status, operations), nil
	}).AnyTimes()

	infra.commandUC.TransactionRedisRepo = redisRepo
	infra.handler.Query.TransactionRedisRepo = redisRepo
}

func pendingCacheBalanceResult(status string, operations []mmodel.BalanceOperation) *mmodel.BalanceAtomicResult {
	before := make([]*mmodel.Balance, 0, len(operations))
	after := make([]*mmodel.Balance, 0, len(operations))
	seen := make(map[string]struct{}, len(operations))

	for _, balanceOperation := range operations {
		balance := balanceOperation.Balance
		if balance == nil {
			continue
		}

		if _, ok := seen[balance.ID]; ok {
			continue
		}
		seen[balance.ID] = struct{}{}

		isSource := strings.Contains(balance.Alias, "source")
		if status == constant.PENDING && !isSource {
			continue
		}
		if status == constant.CANCELED && !isSource {
			continue
		}

		beforeBalance := *balance
		afterBalance := *balance

		switch status {
		case constant.PENDING:
			afterBalance.Available = balance.Available.Sub(decimal.NewFromInt(100))
			afterBalance.OnHold = decimal.NewFromInt(100)
			afterBalance.Version = balance.Version + 1
		case constant.APPROVED:
			if isSource {
				beforeBalance.Available = decimal.NewFromInt(900)
				beforeBalance.OnHold = decimal.NewFromInt(100)
				beforeBalance.Version = 1
				afterBalance = beforeBalance
				afterBalance.OnHold = decimal.Zero
				afterBalance.Version = 2
			} else {
				afterBalance.Available = balance.Available.Add(decimal.NewFromInt(100))
				afterBalance.Version = balance.Version + 1
			}
		case constant.CANCELED:
			beforeBalance.Available = decimal.NewFromInt(900)
			beforeBalance.OnHold = decimal.NewFromInt(100)
			beforeBalance.Version = 1
			afterBalance = beforeBalance
			afterBalance.Available = decimal.NewFromInt(1000)
			afterBalance.OnHold = decimal.Zero
			afterBalance.Version = 2
		}

		before = append(before, &beforeBalance)
		after = append(after, &afterBalance)
	}

	return &mmodel.BalanceAtomicResult{Before: before, After: after}
}

type pendingCacheHTTPResponse struct {
	status int
	body   []byte
}

func postPendingCacheRequest(t *testing.T, infra *testAsyncInfra, path, body string) pendingCacheHTTPResponse {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return pendingCacheHTTPResponse{status: resp.StatusCode, body: responseBody}
}

func getPendingCacheTransaction(infra *testAsyncInfra, transactionID uuid.UUID) (transaction.Transaction, string, bool) {
	path := "/v1/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() +
		"/transactions/" + transactionID.String()
	req := httptest.NewRequest(nethttp.MethodGet, path, nil)

	resp, err := infra.app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		return transaction.Transaction{}, "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
		return transaction.Transaction{}, resp.Header.Get("X-Cache-Hit"), false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return transaction.Transaction{}, resp.Header.Get("X-Cache-Hit"), false
	}

	var tran transaction.Transaction
	if err := json.Unmarshal(body, &tran); err != nil {
		return transaction.Transaction{}, resp.Header.Get("X-Cache-Hit"), false
	}

	return tran, resp.Header.Get("X-Cache-Hit"), true
}

func seedPendingCacheBalance(t *testing.T, infra *testAsyncInfra, alias string, available decimal.Decimal) {
	t.Helper()

	params := postgrestestutil.DefaultBalanceParams()
	params.Alias = alias
	params.AssetCode = "USD"
	params.Available = available
	params.OnHold = decimal.Zero

	postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID,
		uuid.Must(libCommons.GenerateUUIDv7()), params)
}

func pendingCacheOperationIDs(operations []*operation.Operation) []string {
	ids := make([]string, 0, len(operations))

	for _, op := range operations {
		if op != nil {
			ids = append(ids, op.ID)
		}
	}

	return ids
}

func pendingCachePersistedOperationIDs(db *sql.DB, transactionID uuid.UUID) ([]string, error) {
	rows, err := db.Query(`SELECT id FROM operation WHERE transaction_id = $1`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}
