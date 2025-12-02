package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
	"github.com/shopspring/decimal"
)

// Pending outflow -> commit -> revert must succeed; uses isolation helpers and cache-aware seed.
func TestIntegration_Transactions_PendingCommitThenRevert_Succeeds(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	iso := h.NewTestIsolation()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := iso.MakeTestHeaders()

	// Setup: org -> ledger -> USD asset -> account
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("Ledger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("asset: %v", err)
	}
	alias := iso.UniqueAccountAlias("rev")
	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Seed 10 with unique transaction code and wait briefly until observed (cache-aware)
	seedPayload := map[string]any{
		"code": iso.UniqueTransactionCode("SEED"),
		"send": map[string]any{
			"asset": "USD",
			"value": "10.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "10.00"},
				}},
			},
		},
	}
	seedPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID)
	if code, body, err := trans.Request(ctx, "POST", seedPath, headers, seedPayload); err != nil || code != 201 {
		t.Fatalf("seed inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}
	seedDeadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			seedDeadline = time.Now().Add(d)
		}
	}
	for {
		cur, err := h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
		if err == nil && cur.Equal(decimal.RequireFromString("10.00")) {
			break
		}
		if time.Now().After(seedDeadline) {
			t.Fatalf("seed mismatch: want=10.00 not observed")
		}
		time.Sleep(75 * time.Millisecond)
	}

	// Create pending outflow 3.00 with unique code
	p := map[string]any{
		"code":    iso.UniqueTransactionCode("OUT-PENDING"),
		"pending": true,
		"send": map[string]any{
			"asset":  "USD",
			"value":  "3.00",
			"source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}},
		},
	}
	code, body, err := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", orgID, ledgerID), headers, p)
	if err != nil || code != 201 {
		t.Fatalf("pending outflow: %d %s", code, string(body))
	}
	var tx struct {
		ID     string `json:"id"`
		Status struct {
			Code string `json:"code"`
		} `json:"status"`
	}
	if e := json.Unmarshal(body, &tx); e != nil || tx.ID == "" {
		t.Fatalf("parse tx id: %v body=%s", e, string(body))
	}

	// Ensure transaction is retrievable before commit (handles minor propagation windows)
	txGetPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, tx.ID)
	getDeadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			getDeadline = time.Now().Add(d)
		}
	}
	for {
		c, b, e := trans.Request(ctx, "GET", txGetPath, headers, nil)
		if e == nil && c == 200 {
			break
		}
		if time.Now().After(getDeadline) {
			t.Fatalf("transaction not retrievable before commit: last_status=%d body=%s err=%v", c, string(b), e)
		}
		time.Sleep(75 * time.Millisecond)
	}

	// Commit must return 200/201
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/commit", orgID, ledgerID, tx.ID), headers, nil)
	if err != nil || (code != 200 && code != 201) {
		t.Fatalf("commit expected 200/201 got %d body=%s err=%v", code, string(body), err)
	}

	// Wait until transaction status becomes APPROVED before revert
	approveDeadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			approveDeadline = time.Now().Add(d)
		}
	}
	for {
		c, b, e := trans.Request(ctx, "GET", txGetPath, headers, nil)
		if e == nil && c == 200 {
			var got struct {
				Status struct {
					Code string `json:"code"`
				} `json:"status"`
			}
			if json.Unmarshal(b, &got) == nil && got.Status.Code == "APPROVED" {
				break
			}
		}
		if time.Now().After(approveDeadline) {
			t.Fatalf("transaction not APPROVED before revert: last_body=%s", string(b))
		}
		time.Sleep(75 * time.Millisecond)
	}

	// Revert must return 200/201
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/revert", orgID, ledgerID, tx.ID), headers, nil)
	if err != nil || (code != 200 && code != 201) {
		t.Fatalf("revert expected 200/201 got %d body=%s err=%v", code, string(body), err)
	}

	// Verify functional outcome: balance restored to initial 10.00
	revertDeadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			revertDeadline = time.Now().Add(d)
		}
	}
	var last decimal.Decimal
	for {
		cur, e := h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
		if e == nil {
			last = cur
			if cur.Equal(decimal.RequireFromString("10.00")) {
				break
			}
		}
		if time.Now().After(revertDeadline) {
			t.Fatalf("revert did not restore balance; want=10.00 got=%s", last.String())
		}
		time.Sleep(75 * time.Millisecond)
	}
}
