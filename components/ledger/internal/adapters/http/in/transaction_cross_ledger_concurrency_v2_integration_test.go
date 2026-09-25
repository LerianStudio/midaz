// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type crossLedgerGroupRaceResult struct {
	status   int
	replayed string
	groupID  string
	code     string
	body     string
	err      error
}

func fireCrossLedgerGroupRequest(app *fiber.App, url string) crossLedgerGroupRaceResult {
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		return crossLedgerGroupRaceResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return crossLedgerGroupRaceResult{status: resp.StatusCode, err: err}
	}
	var decoded struct {
		GroupID string `json:"groupId"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal(raw, &decoded)
	return crossLedgerGroupRaceResult{
		status: resp.StatusCode, replayed: resp.Header.Get("X-Idempotency-Replayed"),
		groupID: decoded.GroupID, code: decoded.Code, body: string(raw),
	}
}

func TestIntegration_CrossLedgerRevert_ConcurrentMembers(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	ledgerA, ledgerB := fixture.newLedger(t), fixture.newLedger(t)
	if ledgerA.String() > ledgerB.String() {
		ledgerA, ledgerB = ledgerB, ledgerA
	}
	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)
	sourceBalanceID, _ := seedFundedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID,
		ledgerA, "@race-source", "@external/USD", 1000, 1000)
	_, destinationBalanceID := seedFundedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID,
		ledgerB, "@external/USD", "@race-destination", 1000, 1000)
	request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "concurrent group revert", "@race-source", "@race-destination", 100)
	request.Credits[0].LedgerID = ledgerB.String()
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	originResponse := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "group-race-origin")
	originBody := drainBody(t, originResponse)
	require.Equal(t, http.StatusCreated, originResponse.StatusCode, string(originBody))
	var origin CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(originBody, &origin))
	require.Len(t, origin.Transactions, 2)

	counted := &countingAtomicBatchEngine{delegate: fixture.engine}
	fixture.infra.handler.Command.Engine = counted
	urls := []string{
		v2RevertURL(fixture.infra.orgID, ledgerA, uuid.MustParse(origin.Transactions[0].ID)),
		v2RevertURL(fixture.infra.orgID, ledgerB, uuid.MustParse(origin.Transactions[1].ID)),
	}
	results := make([]crossLedgerGroupRaceResult, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = fireCrossLedgerGroupRequest(fixture.app, urls[index%2])
		}(index)
	}
	close(start)
	wg.Wait()

	fresh, replay := 0, 0
	var winningGroup string
	for _, result := range results {
		require.NoError(t, result.err)
		switch result.status {
		case http.StatusCreated:
			if result.replayed == "true" {
				replay++
			} else {
				fresh++
				winningGroup = result.groupID
			}
		case http.StatusConflict:
			require.Contains(t, []string{constant.ErrIdempotencyKey.Error(), constant.ErrTransactionIDHasAlreadyParentTransaction.Error()}, result.code, result.body)
		default:
			t.Fatalf("unexpected concurrent revert status %d: %s", result.status, result.body)
		}
	}
	require.Equal(t, 1, fresh, "only one group reversal may apply")
	require.Equal(t, int64(1), counted.calls.Load(), "only one accounting execution may apply")
	for _, result := range results {
		if result.replayed == "true" {
			require.Equal(t, winningGroup, result.groupID)
		}
	}
	t.Logf("one fresh reversal, %d replays, %d conflicts", replay, len(results)-fresh-replay)
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, sourceBalanceID))
	requireDecimalEqual(t, decimal.NewFromInt(1000), postgrestestutil.GetBalanceAvailable(t, fixture.infra.pgContainer.DB, destinationBalanceID))
	requireCachedAvailable(t, fixture, ledgerA, "@race-source", 1000)
	requireCachedAvailable(t, fixture, ledgerB, "@race-destination", 1000)
	require.Equal(t, 2, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
	require.Equal(t, 2, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
}

func TestIntegration_CrossLedgerDirect_ConcurrentScopeConflict(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	ledgerA, ledgerB := fixture.newLedger(t), fixture.newLedger(t)
	if ledgerA.String() < ledgerB.String() {
		ledgerA, ledgerB = ledgerB, ledgerA
	}
	fixture.setCrossLedgerEnabled(t, ledgerA, true)
	fixture.setCrossLedgerEnabled(t, ledgerB, true)
	seedFundedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID,
		ledgerA, "@scope-a", "@external/USD", 1000, 1000)
	seedFundedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID,
		ledgerB, "@external/USD", "@scope-b", 1000, 1000)
	forward := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "forward", "@scope-a", "@scope-b", 10)
	forward.Credits[0].LedgerID = ledgerB.String()
	forwardRaw, err := json.Marshal(forward)
	require.NoError(t, err)
	first := postTransaction(t, fixture.app, v2CreateURL("direct"), string(forwardRaw), "same-cross-ledger-key")
	firstBody := drainBody(t, first)
	require.Equal(t, http.StatusCreated, first.StatusCode, string(firstBody))
	var created CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(firstBody, &created))

	replay := postTransaction(t, fixture.app, v2CreateURL("direct"), string(forwardRaw), "same-cross-ledger-key")
	replayBody := drainBody(t, replay)
	require.Equal(t, http.StatusCreated, replay.StatusCode, string(replayBody))
	require.Equal(t, "true", replay.Header.Get("X-Idempotency-Replayed"))
	require.JSONEq(t, string(firstBody), string(replayBody))

	reverse := atomicBatchTransfer(fixture.infra.orgID, ledgerB, "reverse", "@scope-b", "@scope-a", 10)
	reverse.Credits[0].LedgerID = ledgerA.String()
	reverseRaw, err := json.Marshal(reverse)
	require.NoError(t, err)
	second := postTransaction(t, fixture.app, v2CreateURL("direct"), string(reverseRaw), "same-cross-ledger-key")
	secondBody := drainBody(t, second)
	require.Equal(t, http.StatusConflict, second.StatusCode, string(secondBody))
	requireProblemCode(t, secondBody, constant.ErrIdempotencyKey.Error())
	require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerA))
	require.Equal(t, 1, countTransactionsInLedger(t, fixture.infra.pgContainer.DB, ledgerB))
	requireCachedAvailable(t, fixture, ledgerA, "@scope-a", 990)
	requireCachedAvailable(t, fixture, ledgerB, "@scope-b", 1010)
}

func TestIntegration_CrossLedgerCommit_ConcurrentMembers(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)
	ledgerA, ledgerB, ledgerC := fixture.newLedger(t), fixture.newLedger(t), fixture.newLedger(t)
	if ledgerA.String() < ledgerB.String() {
		ledgerA, ledgerB = ledgerB, ledgerA
	}
	if ledgerA.String() < ledgerC.String() {
		ledgerA, ledgerC = ledgerC, ledgerA
	}
	for _, ledgerID := range []uuid.UUID{ledgerA, ledgerB, ledgerC} {
		fixture.setCrossLedgerEnabled(t, ledgerID, true)
	}
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, "@commit-a", "@external/USD", 100)
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerC, "@commit-c", "@external/USD", 100)
	seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", "@commit-destination", 100)
	request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "concurrent commit", "@commit-a", "@commit-destination", 100)
	request.Debits[0].Amount = "50"
	request.Debits = append(request.Debits, TransactionV2LegRequest{
		Alias: "@commit-c", OrganizationID: fixture.infra.orgID.String(), LedgerID: ledgerC.String(), Amount: "50",
	})
	request.Credits[0].LedgerID = ledgerB.String()
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	hold := postTransaction(t, fixture.app, v2CreateURL("hold"), string(raw), "concurrent-group-commit-hold")
	holdBody := drainBody(t, hold)
	require.Equal(t, http.StatusCreated, hold.StatusCode, string(holdBody))
	var held CreateTransactionV2Response
	require.NoError(t, json.Unmarshal(holdBody, &held))
	require.Len(t, held.Transactions, 2)
	counted := &countingAtomicBatchEngine{delegate: fixture.engine}
	fixture.infra.handler.Command.Engine = counted
	urls := []string{
		v2CommitURL(fixture.infra.orgID, uuid.MustParse(held.Transactions[0].LedgerID), uuid.MustParse(held.Transactions[0].ID)),
		v2CommitURL(fixture.infra.orgID, uuid.MustParse(held.Transactions[1].LedgerID), uuid.MustParse(held.Transactions[1].ID)),
	}
	results := make([]crossLedgerGroupRaceResult, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = fireCrossLedgerGroupRequest(fixture.app, urls[index])
		}(index)
	}
	close(start)
	wg.Wait()
	fresh := 0
	for _, result := range results {
		require.NoError(t, result.err)
		if result.status == http.StatusCreated && result.replayed != "true" {
			fresh++
			continue
		}
		if result.status == http.StatusCreated {
			require.Equal(t, *held.GroupID, result.groupID)
			continue
		}
		require.Equal(t, http.StatusConflict, result.status, result.body)
	}
	require.Equal(t, 1, fresh)
	require.Equal(t, int64(1), counted.calls.Load())
	requireCachedAvailable(t, fixture, ledgerB, "@commit-destination", 100)
}
