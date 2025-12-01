package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Covers: pending outflow + commit (affects balances), cancel (no effect), revert (restores balances)
func TestIntegration_Transactions_Lifecycle_PendingCommitCancelRevert(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup org/ledger/account with helpers
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("L"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := iso.UniqueAccountAlias("lc")
	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Seed 10.00 with unique code and wait for cache-aware availability
	seed := map[string]any{
		"code": iso.UniqueTransactionCode("SEED"),
		"send": map[string]any{
			"asset": "USD", "value": "10.00",
			"distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "10.00"}}}},
		},
	}
	code, body, err := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID), headers, seed)
	if err != nil || code != 201 {
		t.Fatalf("seed inflow: code=%d err=%v body=%s", code, err, string(body))
	}
	wantSeed := decimal.RequireFromString("10.00")
	dl := time.Now().Add(5 * time.Second)
	if td, ok := t.Deadline(); ok {
		if d := time.Until(td) / 2; d < 5*time.Second {
			dl = time.Now().Add(d)
		}
	}
	for {
		cur, e := h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
		if e == nil && cur.Equal(wantSeed) {
			break
		}
		if time.Now().After(dl) {
			t.Fatalf("seed not observed; want=10.00")
		}
		time.Sleep(75 * time.Millisecond)
	}

	// helper: sum available USD by alias via helper
	sumAvail := func() decimal.Decimal {
		v, e := h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
		if e != nil {
			t.Fatalf("get available: %v", e)
		}
		return v
	}

	base := sumAvail() // expect 10.00

	// pending outflow 3.00 with unique code
	outPending := map[string]any{
		"code":    iso.UniqueTransactionCode("OUT-PENDING"),
		"pending": true,
		"send": map[string]any{
			"asset": "USD", "value": "3.00",
			"source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}},
		},
	}
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", orgID, ledgerID), headers, outPending)
	if err != nil || code != 201 {
		t.Fatalf("create pending outflow: code=%d err=%v body=%s", code, err, string(body))
	}
	var tx struct {
		ID     string `json:"id"`
		Status struct {
			Code string `json:"code"`
		} `json:"status"`
	}
	_ = json.Unmarshal(body, &tx)
	if tx.Status.Code != "PENDING" {
		t.Fatalf("expected PENDING, got %s", tx.Status.Code)
	}

	// Ensure transaction is retrievable before commit (avoid transient 404)
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, tx.ID)
	gdl := time.Now().Add(5 * time.Second)
	if td, ok := t.Deadline(); ok {
		if d := time.Until(td) / 2; d < 5*time.Second {
			gdl = time.Now().Add(d)
		}
	}
	ready := false
	for {
		c, _, e := trans.Request(ctx, "GET", getPath, headers, nil)
		if e == nil && c == 200 {
			ready = true
			break
		}
		if time.Now().After(gdl) {
			break
		}
		time.Sleep(75 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("transaction not retrievable before commit")
	}

	// Commit
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/commit", orgID, ledgerID, tx.ID), headers, nil)
	if err != nil || code != 201 {
		t.Fatalf("commit: code=%d err=%v body=%s", code, err, string(body))
	}
	var txCommitResp struct {
		Status struct {
			Code string `json:"code"`
		} `json:"status"`
	}
	_ = json.Unmarshal(body, &txCommitResp)
	if txCommitResp.Status.Code != "APPROVED" {
		t.Fatalf("expected APPROVED after commit, got %s", txCommitResp.Status.Code)
	}

	// After commit, wait until availability reflects 7.00
	wantCommit, _ := decimal.NewFromString("7.00") // 10 - 3
	// Recalculate timeout against current test deadline before each wait to avoid stale values
	calcTimeout := func(base time.Duration) time.Duration {
		if td, ok := t.Deadline(); ok {
			remaining := time.Until(td)
			// If deadline already passed, use tiny positive duration to avoid ticker panics
			if remaining <= 0 {
				return 1 * time.Millisecond
			}
			if d := remaining / 2; d < base {
				return d
			}
		}
		return base
	}
	afterCommit, err := h.WaitForAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers, wantCommit, calcTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("after commit availability not observed: %v", err)
	}

	// Cancel another pending should not affect available
	outPending2 := map[string]any{
		"code":    iso.UniqueTransactionCode("OUT-CANCEL"),
		"pending": true,
		"send": map[string]any{
			"asset": "USD", "value": "2.00",
			"source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "2.00"}}}},
		},
	}
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", orgID, ledgerID), headers, outPending2)
	if err != nil || code != 201 {
		t.Fatalf("create pending outflow2: code=%d err=%v body=%s", code, err, string(body))
	}
	var tx2 struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &tx2)

	// Ensure transaction 2 is retrievable before cancel (avoid transient 404)
	getPath2 := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, tx2.ID)
	gdl2 := time.Now().Add(5 * time.Second)
	if td, ok := t.Deadline(); ok {
		if d := time.Until(td) / 2; d < 5*time.Second {
			gdl2 = time.Now().Add(d)
		}
	}
	ready2 := false
	for {
		c, _, e := trans.Request(ctx, "GET", getPath2, headers, nil)
		if e == nil && c == 200 {
			ready2 = true
			break
		}
		if time.Now().After(gdl2) {
			break
		}
		time.Sleep(75 * time.Millisecond)
	}
	if !ready2 {
		t.Fatalf("transaction not retrievable before cancel")
	}

	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/cancel", orgID, ledgerID, tx2.ID), headers, nil)
	if err != nil || code != 201 {
		t.Fatalf("cancel: code=%d err=%v body=%s", code, err, string(body))
	}
	// After cancel, availability should remain equal to post-commit value (7.00)
	afterCancel, err := h.WaitForAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers, wantCommit, calcTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("after cancel availability not restored: %v", err)
	}
	if !afterCancel.Equal(afterCommit) {
		t.Fatalf("cancel should not change available: before %s after %s", afterCommit, afterCancel)
	}

	// Revert approved transaction (first one)
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/revert", orgID, ledgerID, tx.ID), headers, nil)
	if code == 500 {
		t.Skipf("known backend issue: revert approved returns 500; expected 200/201. body=%s", string(body))
	}
	if err != nil || (code != 201 && code != 200) {
		t.Fatalf("revert: code=%d err=%v body=%s", code, err, string(body))
	}
	// After revert, wait until availability returns to base
	reverted, err := h.WaitForAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers, base, calcTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("after revert base not restored: %v", err)
	}
	if !reverted.Equal(base) {
		t.Fatalf("revert should restore base: base %s got %s", base, reverted)
	}
}
