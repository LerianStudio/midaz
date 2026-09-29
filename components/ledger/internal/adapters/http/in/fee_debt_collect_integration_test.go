// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	nethttp "net/http"
	"testing"
	"time"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	feesmongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// newFeeDebtCollectApp mounts the /v2 standalone collection over the harness.
func (h *feeHarness) newFeeDebtCollectApp() *fiber.App {
	app := fiber.New()

	libProblem.Install()
	http.InstallHumaFrameworkErrors()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-fee-collect", Version: "test", Servers: []string{"/v2"}})
	http.InstallLedgerSchemaNamer(hAPI)
	RegisterFeeDebtCollectV2RoutesToApp(apiV2, hAPI, &authMiddleware.AuthClient{Enabled: false}, h.handler, nil)

	return app
}

// enableFeeDebtCollect wires the engine, the Fees record, revert evidence and a
// deferrable fee applier onto the harness, and returns the record.
func (h *feeHarness) enableFeeDebtCollect(t *testing.T) *fee_debt.Repository {
	t.Helper()
	h.enableAccountingEngine(t)

	feeDebts, err := fee_debt.NewRepository(&feesmongo.MongoConnection{Database: "test_db", DB: h.mongoContainer.Client}, nil)
	require.NoError(t, err)

	engineRedis, ok := h.redisRepo.(*redistransaction.RedisConsumerRepository)
	require.True(t, ok)

	h.commandUC.FeeDebts = feeDebts
	h.commandUC.AppliedTransactionCompleter = command.NewTransactionCompletionService(h.completionStore, h.metaRepo).WithFeeDebtRecorder(feeDebts)
	h.queryUC.EngineWriteBehindCodec = command.EngineWriteBehindEvidenceCodec{}
	h.commandUC.TransactionEvidenceResolver = testEngineEvidenceResolver{repository: engineRedis}
	h.commandUC.EngineRecoveryAcknowledger = &atomicBatchHTTPRecoveryAcknowledger{repository: engineRedis, completedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}

	return feeDebts
}

// owedByPayer is what @payer owes by the Fees record and by the live list.
func (h *feeHarness) owedByPayer(t *testing.T, feeDebts *fee_debt.Repository) []string {
	t.Helper()

	recorded, err := feeDebts.OpenTotal(h.ctx(), h.orgID, h.ledgerID, "@payer#default")
	require.NoError(t, err)

	seeds, err := h.queryUC.GetFeeDebtSeeds(h.ctx(), h.orgID, h.ledgerID, []string{"@payer#default"})
	require.NoError(t, err)

	live := decimal.Zero
	for _, item := range seeds["@payer#default"] {
		live = live.Add(item.Remaining)
	}

	return []string{recorded.String(), live.String()}
}

// TestFeeDebtCollect proves a standalone collection settles the debt a /v1 credit left
// open, keeps the Fees record in step with the live list, and cannot be reverted.
func TestFeeDebtCollect(t *testing.T) {
	h := setupFeeHarness(t)
	feeDebts := h.enableFeeDebtCollect(t)
	v1App, v2App, collectApp := h.newApp(), h.newV2App(), h.newFeeDebtCollectApp()

	h.seedBalance(t, "@payer", "BRL", decimal.NewFromInt(100), "deposit")
	h.seedBalance(t, "@receiver", "BRL", decimal.Zero, "deposit")
	h.seedBalance(t, "@funder", "BRL", decimal.NewFromInt(1000), "deposit")
	h.seedBalance(t, "@fees", "BRL", decimal.Zero, "deposit")
	fee := flatFee("deferred_fee", "@fees", "10", false)
	fee.deferrable = true
	h.seedPackage(t, packageSpec{label: "deferred", metadataSelector: map[string]string{"fee": "deferred"}, fees: []feeSpec{fee}})

	body := h.v2WithMetadata(h.v2Body("defer", "BRL", "100", []string{h.v2Leg("@payer", "100")}, []string{h.v2Leg("@receiver", "100")}), `{"fee":"deferred"}`)
	created := h.createV2Direct(t, v2App, body, nil)
	require.Equalf(t, nethttp.StatusCreated, created.status, "deferred transfer: %s", created.rawBody)

	credit := h.createJSON(t, v1App, `{"description":"v1 credit","send":{"asset":"BRL","value":"30",
		"source":{"from":[{"accountAlias":"@funder","amount":{"asset":"BRL","value":"30"}}]},
		"distribute":{"to":[{"accountAlias":"@payer","amount":{"asset":"BRL","value":"30"}}]}}}`, nil)
	require.Equalf(t, nethttp.StatusCreated, credit.status, "v1 credit: %s", credit.rawBody)

	path := "/v2/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String() + "/fee-debts/collect"
	require.Equal(t, []string{"10", "10"}, h.owedByPayer(t, feeDebts), "a /v1 credit settles no debt")

	capped, keyed := `{"accountAlias":"@payer","maxAmount":"4"}`, map[string]string{"X-Idempotency": "collect-once", "X-TTL": "60"}
	first := h.post(t, collectApp, path, capped, keyed)
	require.Equalf(t, nethttp.StatusOK, first.status, "capped collect: %s", first.rawBody)
	require.Equal(t, []any{"4", "false"}, []any{first.body["collected"], first.replayed})
	collectionID, _ := first.body["transactionId"].(string)
	require.NotEmpty(t, collectionID)

	retry := h.post(t, collectApp, path, capped, keyed)
	for attempt := 0; retry.status == nethttp.StatusConflict && attempt < 50; attempt++ {
		time.Sleep(20 * time.Millisecond) // the first answer is stored asynchronously
		retry = h.post(t, collectApp, path, capped, keyed)
	}

	require.Equalf(t, nethttp.StatusOK, retry.status, "keyed retry: %s", retry.rawBody)
	require.Equal(t, []any{first.body, "true"}, []any{retry.body, retry.replayed}, "a keyed retry replays the first answer")

	var amount string
	require.NoError(t, h.db.QueryRow(`SELECT amount::text FROM transaction WHERE id = $1`, collectionID).Scan(&amount))
	require.True(t, decimal.RequireFromString(amount).Equal(decimal.NewFromInt(4)), "the transaction amount is what it settled")

	require.Equal(t, []string{"6", "6"}, h.owedByPayer(t, feeDebts), "the Fees record keeps the live remaining")

	for range 2 {
		status, out := driveFeeV2(t, collectApp, nethttp.MethodPost, path, `{"accountAlias":"@payer","maxAmount":"1"}`)
		require.Equalf(t, nethttp.StatusOK, status, "keyless collect: %v", out)
		require.Equal(t, "1", out["collected"], "identical keyless collects stay distinct")
	}

	status, out := driveFeeV2(t, collectApp, nethttp.MethodPost, path, `{"accountAlias":"@payer","balanceKey":"default"}`)
	require.Equalf(t, nethttp.StatusOK, status, "full collect: %v", out)
	require.Equal(t, "4", out["collected"])

	require.Equal(t, []string{"0", "0"}, h.owedByPayer(t, feeDebts))

	status, out = driveFeeV2(t, collectApp, nethttp.MethodPost, path, `{"accountAlias":"@payer"}`)
	require.Equalf(t, nethttp.StatusOK, status, "nothing owed: %v", out)
	require.Equal(t, map[string]any{"collected": "0"}, out)

	reverted := h.post(t, v2App, h.v2StatePath(uuid.MustParse(collectionID), "revert"), "", nil)
	require.Equalf(t, nethttp.StatusUnprocessableEntity, reverted.status, "revert of a collection: %s", reverted.rawBody)
	require.Equal(t, "0089", reverted.body["code"])

	status, out = driveFeeV2(t, collectApp, nethttp.MethodPost, path, `{"accountAlias":"@nobody"}`)
	require.Equalf(t, nethttp.StatusNotFound, status, "unknown balance: %v", out)
	require.Equal(t, "0007", out["code"])

	status, out = driveFeeV2(t, collectApp, nethttp.MethodPost, path, `{"accountAlias":"@payer","maxAmount":"0"}`)
	require.Equalf(t, nethttp.StatusBadRequest, status, "non-positive maxAmount: %v", out)
	require.Equal(t, "0047", out["code"])
}
