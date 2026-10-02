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
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// newAccountAdminV2App mounts the /v2 account and balance surfaces over the harness.
func (h *feeHarness) newAccountAdminV2App() *fiber.App {
	app := fiber.New()

	libProblem.Install()
	http.InstallHumaFrameworkErrors()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	apiV2 := app.Group("/v2")
	hAPI := openapi.New(app, apiV2, openapi.Config{Title: "ledger-fee-creditor", Version: "test", Servers: []string{"/v2"}})
	http.InstallLedgerSchemaNamer(hAPI)

	auth := &authMiddleware.AuthClient{Enabled: false}
	RegisterAccountV2RoutesToApp(apiV2, hAPI, auth, &AccountHandler{Command: h.commandUC, Query: h.queryUC}, nil)
	RegisterBalanceV2RoutesToApp(apiV2, hAPI, auth, &BalanceHandler{Command: h.commandUC, Query: h.queryUC}, nil)

	return app
}

// TestFeeDebtCreditorGuard proves a fee account that an open fee debt names as creditor
// can be neither closed nor deleted, balance or account, until the debt is settled.
func TestFeeDebtCreditorGuard(t *testing.T) {
	h := setupFeeHarness(t)

	feeDebts, err := fee_debt.NewRepository(&feesmongo.MongoConnection{Database: "test_db", DB: h.mongoContainer.Client}, nil)
	require.NoError(t, err)

	h.commandUC.FeeDebts = feeDebts

	redisRepository, ok := h.redisRepo.(*redis.RedisConsumerRepository)
	require.True(t, ok)

	projectAndAcknowledge := func() {
		h.commandUC.AppliedTransactionCompleter = command.NewTransactionCompletionService(h.completionStore, h.metaRepo).
			WithFeeDebtRecorder(feeDebts)
		h.commandUC.EngineRecoveryAcknowledger = &atomicBatchHTTPRecoveryAcknowledger{
			repository: redisRepository, completedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
		}
	}

	txApp, adminApp := h.newV2App(), h.newAccountAdminV2App()

	h.seedBalance(t, "@payer", "BRL", decimal.NewFromInt(100), "deposit")
	h.seedBalance(t, "@receiver", "BRL", decimal.Zero, "deposit")
	h.seedBalance(t, "@funder", "BRL", decimal.NewFromInt(1000), "deposit")
	feeBalanceID := h.seedBalance(t, "@fees", "BRL", decimal.Zero, "deposit")
	h.seedBalance(t, "@late-payer", "BRL", decimal.NewFromInt(100), "deposit")
	lateFeeBalanceID := h.seedBalance(t, "@late-fees", "BRL", decimal.Zero, "deposit")

	for label, creditor := range map[string]string{"deferred": "@fees", "late": "@late-fees"} {
		fee := flatFee(label+"_fee", creditor, "10", false)
		fee.deferrable = true
		h.seedPackage(t, packageSpec{label: label, metadataSelector: map[string]string{"fee": label}, fees: []feeSpec{fee}})
	}

	var feeAccountID uuid.UUID
	require.NoError(t, h.db.QueryRow(`SELECT account_id FROM balance WHERE id = $1`, feeBalanceID).Scan(&feeAccountID))

	transfer := func(from, to, amount, metadata string) {
		t.Helper()

		body := h.v2Body("creditor guard", "BRL", amount, []string{h.v2Leg(from, amount)}, []string{h.v2Leg(to, amount)})
		if metadata != "" {
			body = h.v2WithMetadata(body, metadata)
		}

		created := h.createV2Direct(t, txApp, body, nil)
		require.Equalf(t, nethttp.StatusCreated, created.status, "transfer %s -> %s: %s", from, to, created.rawBody)
	}
	openTotal := func() string {
		t.Helper()

		total, err := feeDebts.OpenTotal(h.ctx(), h.orgID, h.ledgerID, "@payer#default")
		require.NoError(t, err)

		return total.String()
	}

	scope := "/v2/organizations/" + h.orgID.String() + "/ledgers/" + h.ledgerID.String()
	closePath := scope + "/accounts/" + feeAccountID.String() + "/close"
	requests := []struct{ name, method, path string }{
		{"close the account", nethttp.MethodPost, closePath},
		{"delete the balance", nethttp.MethodDelete, scope + "/balances/" + feeBalanceID.String()},
		{"delete the account", nethttp.MethodDelete, scope + "/accounts/" + feeAccountID.String()},
	}

	// A completion neither projected nor acknowledged leaves the debt only in its engine
	// recovery record, which alone refuses the delete.
	transfer("@late-payer", "@receiver", "100", `{"fee":"late"}`)
	status, body := driveFeeV2(t, adminApp, nethttp.MethodDelete, scope+"/balances/"+lateFeeBalanceID.String(), "")
	require.Equalf(t, nethttp.StatusUnprocessableEntity, status, "in-flight debt: %v", body)
	require.Equalf(t, "0528", body["code"], "in-flight debt: %v", body)

	projectAndAcknowledge()
	transfer("@payer", "@receiver", "100", `{"fee":"deferred"}`)
	require.Equal(t, "10", openTotal(), "the unfunded fee opens a debt owed to @fees")
	drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)

	for _, request := range requests {
		status, body := driveFeeV2(t, adminApp, request.method, request.path, "")
		require.Equalf(t, nethttp.StatusUnprocessableEntity, status, "%s: %v", request.name, body)
		require.Equalf(t, "0528", body["code"], "%s: %v", request.name, body)
	}

	transfer("@funder", "@payer", "10", "")
	require.Equal(t, "0", openTotal(), "the credit settles the debt into @fees")
	transfer("@fees", "@funder", "10", "")
	drainBalanceSync(t, h.ctx(), h.commandUC, h.redisRepo, h.orgID, h.ledgerID)

	status, body = driveFeeV2(t, adminApp, nethttp.MethodPost, closePath, "")
	require.Equalf(t, nethttp.StatusNoContent, status, "a settled creditor closes: %v", body)
}
