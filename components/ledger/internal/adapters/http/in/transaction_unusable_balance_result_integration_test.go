// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redisadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// When the balance script has already moved money but its answer cannot be
// mapped back to the requested balances, the request must fail without undoing
// the recovery state that a rejected operation would release: a retry must not
// move the balances a second time.
//
// NOT PARALLEL: setupTestInfra and the Huma error hooks share process-global state.

const unusableResultCause = "injected unusable balance result"

// unusableResultAfterScript is the real Redis repository whose balance script
// runs for real, but whose answer is reported as unusable.
type unusableResultAfterScript struct {
	redisadapter.RedisRepository
}

func (r *unusableResultAfterScript) ProcessBalanceAtomicOperation(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, transactionStatus string, pending bool, balances []mmodel.BalanceOperation) (*mmodel.BalanceAtomicResult, error) {
	if _, err := r.RedisRepository.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, transactionID, transactionStatus, pending, balances); err != nil {
		return nil, err
	}

	return nil, &redisadapter.UnusableBalanceResultError{Err: errors.New(unusableResultCause)}
}

const unusableResultPendingBody = `{
	"description":"pending transfer",
	"pending":true,
	"send":{
		"asset":"USD",
		"value":"100",
		"source":{"from":[{"accountAlias":"@src","amount":{"asset":"USD","value":"100"}}]},
		"distribute":{"to":[{"accountAlias":"@dst","amount":{"asset":"USD","value":"100"}}]}
	}
}`

func TestIntegration_TransactionCreate_UnusableBalanceResultKeepsRecoveryState(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	ctx := context.Background()
	app := buildHumaTransactionApp(t, infra.handler, true)
	url := v1JSONURL(infra.orgID, infra.ledgerID)
	realRepo := infra.handler.Command.TransactionRedisRepo

	infra.handler.Command.TransactionRedisRepo = &unusableResultAfterScript{RedisRepository: realRepo}

	resp := postTransaction(t, app, url, equivalentV1Body, "unusable-result-create")
	body := readUnusableResultBody(t, resp)

	assert.Equal(t, nethttp.StatusInternalServerError, resp.StatusCode, "body: %s", body)
	assert.NotContains(t, body, unusableResultCause, "the internal cause must not reach the client")

	source := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@src", "default")
	require.NotNil(t, source, "the script must have written the source balance")
	assert.True(t, source.Available.Equal(decimal.NewFromInt(900)), "the script moved the money, got %s", source.Available)

	assert.Equal(t, 0, countTransactionsInLedger(t, infra.pgContainer.DB, infra.ledgerID), "no transaction may be persisted without its operations")
	assert.Equal(t, 1, countBackupEntriesInLedger(t, ctx, realRepo, infra.orgID, infra.ledgerID), "the backup entry must stay for recovery")

	infra.handler.Command.TransactionRedisRepo = realRepo

	retry := postTransaction(t, app, url, equivalentV1Body, "unusable-result-create")
	retryBody := readUnusableResultBody(t, retry)

	assert.Equal(t, nethttp.StatusConflict, retry.StatusCode, "the kept idempotency claim must stop a retry: %s", retryBody)
	assert.Contains(t, retryBody, "0084")

	source = getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@src", "default")
	assert.True(t, source.Available.Equal(decimal.NewFromInt(900)), "a retry must not move the money again, got %s", source.Available)
}

func TestIntegration_TransactionCommit_UnusableBalanceResultKeepsRecoveryState(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")

	infra := setupTestInfra(t)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	seedTransfer(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, "@src", "@dst", 1000)

	ctx := context.Background()
	app := buildHumaTransactionApp(t, infra.handler, true)
	realRepo := infra.handler.Command.TransactionRedisRepo

	created := postTransaction(t, app, v1JSONURL(infra.orgID, infra.ledgerID), unusableResultPendingBody, "")
	createdBody := readUnusableResultBody(t, created)
	require.Equal(t, nethttp.StatusCreated, created.StatusCode, "pending create: %s", createdBody)

	var pending struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(createdBody), &pending))

	commitURL := "/v1/organizations/" + infra.orgID.String() + "/ledgers/" + infra.ledgerID.String() + "/transactions/" + pending.ID + "/commit"

	infra.handler.Command.TransactionRedisRepo = &unusableResultAfterScript{RedisRepository: realRepo}

	commit := postUnusableResultCommit(t, app, commitURL)
	commitBody := readUnusableResultBody(t, commit)

	assert.Equal(t, nethttp.StatusInternalServerError, commit.StatusCode, "body: %s", commitBody)
	assert.NotContains(t, commitBody, unusableResultCause, "the internal cause must not reach the client")

	destination := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dst", "default")
	require.NotNil(t, destination, "the script must have credited the destination")
	assert.True(t, destination.Available.Equal(decimal.NewFromInt(100)), "the script moved the money, got %s", destination.Available)

	assert.Equal(t, 1, countBackupEntriesInLedger(t, ctx, realRepo, infra.orgID, infra.ledgerID), "the commit backup entry must stay for recovery")

	infra.handler.Command.TransactionRedisRepo = realRepo

	retry := postUnusableResultCommit(t, app, commitURL)
	retryBody := readUnusableResultBody(t, retry)

	assert.Contains(t, retryBody, "0486", "the kept pending lock must stop a second commit (status %d)", retry.StatusCode)

	destination = getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dst", "default")
	assert.True(t, destination.Available.Equal(decimal.NewFromInt(100)), "a retry must not credit the destination again, got %s", destination.Available)
}

func postUnusableResultCommit(t *testing.T, app *fiber.App, url string) *nethttp.Response {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodPost, url, nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	return resp
}

func readUnusableResultBody(t *testing.T, resp *nethttp.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return string(body)
}

// countBackupEntriesInLedger counts transaction backup entries whose key is
// scoped to the given ledger.
func countBackupEntriesInLedger(t *testing.T, ctx context.Context, repo redisadapter.RedisRepository, orgID, ledgerID uuid.UUID) int {
	t.Helper()

	entries, err := repo.ReadAllMessagesFromQueue(ctx)
	require.NoError(t, err)

	count := 0

	for key := range entries {
		if strings.Contains(key, orgID.String()) && strings.Contains(key, ledgerID.String()) {
			count++
		}
	}

	return count
}
