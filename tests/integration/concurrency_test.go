package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Parallel inflow/outflow contention on same account without negative balances.
func TestIntegration_ParallelContention_NoNegativeBalance(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	// Create test isolation helper
	isolation := h.NewTestIsolation()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := isolation.MakeTestHeaders()

	// Setup: org, ledger, asset, account
	orgName := isolation.UniqueOrgName("TestContention")
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	ledgerName := isolation.UniqueLedgerName("L")
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": ledgerName})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	alias := isolation.UniqueAccountAlias("acct")
	accPayload := map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias}
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, accPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acc)

	// Create balance tracker
	tracker, err := h.NewOperationTracker(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)
	if err != nil {
		t.Fatalf("create tracker: %v", err)
	}

	// Seed initial balance via inflow: 500.00
	inflow := func(val string) (int, []byte, error) {
		p := map[string]any{
			"code": isolation.UniqueTransactionCode("INF"),
			"send": map[string]any{
				"asset": "USD", "value": val,
				"distribute": map[string]any{
					"to": []map[string]any{{
						"accountAlias": alias,
						"amount":       map[string]any{"asset": "USD", "value": val},
					}},
				},
			},
		}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
	}
	code, body, err = inflow("500.00")
	if err != nil || code != 201 {
		t.Fatalf("seed inflow: code=%d err=%v body=%s", code, err, string(body))
	}

	// Wait for balance to increase by 500.00
	seedAmount := decimal.RequireFromString("500.00")
	if _, err := tracker.VerifyDelta(ctx, seedAmount, 10*time.Second); err != nil {
		t.Fatalf("wait seed balance: %v", err)
	}

	// Prepare parallel operations: 40 outflows of 5.00 (total 200), 20 inflows of 3.00 (total 60)
	var wg sync.WaitGroup
	outSucc := int64(0)
	inSucc := int64(0)
	mu := sync.Mutex{}

	outflow := func(val string) (int, []byte, error) {
		p := map[string]any{
			"code": isolation.UniqueTransactionCode("OUT"),
			"send": map[string]any{
				"asset": "USD", "value": val,
				"source": map[string]any{
					"from": []map[string]any{{
						"accountAlias": alias,
						"amount":       map[string]any{"asset": "USD", "value": val},
					}},
				},
			},
		}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
	}

	// launch outflows
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, _ := outflow("5.00")
			if c == 201 {
				mu.Lock()
				outSucc++
				mu.Unlock()
			}
		}()
	}
	// launch inflows
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, _ := inflow("3.00")
			if c == 201 {
				mu.Lock()
				inSucc++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Calculate expected delta from seeded amount
	// Delta = seed (500) - successful_outflows + successful_inflows
	// Expected = 500 - 40*5 + 20*3 = 500 - 200 + 60 = 360
	// So delta from initial = 360
	expectedDelta := seedAmount.
		Sub(decimal.NewFromInt(int64(outSucc)).Mul(decimal.NewFromInt(5))).
		Add(decimal.NewFromInt(int64(inSucc)).Mul(decimal.NewFromInt(3)))

	// Verify balance changed by expected delta
	got, err := tracker.VerifyDelta(ctx, expectedDelta, 15*time.Second)
	if err != nil {
		actualDelta, _ := tracker.GetCurrentDelta(ctx)
		t.Fatalf("final balance delta mismatch: actual_delta=%s expected_delta=%s err=%v (inSucc=%d outSucc=%d)",
			actualDelta.String(), expectedDelta.String(), err, inSucc, outSucc)
	}

	if got.IsNegative() {
		t.Fatalf("balance went negative: %s", got.String())
	}
}

// Burst of mixed operations with deterministic final balances, and overshoot check avoids negatives.
func TestIntegration_BurstMixedOperations_DeterministicFinal(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	// Create test isolation helper
	isolation := h.NewTestIsolation()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := isolation.MakeTestHeaders()

	// Setup org/ledger/assets/accounts A and B
	orgName := isolation.UniqueOrgName("TestBurst")
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	ledgerName := isolation.UniqueLedgerName("L")
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": ledgerName})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	aAlias := isolation.UniqueAccountAlias("acc-a")
	bAlias := isolation.UniqueAccountAlias("acc-b")
	for _, alias := range []string{aAlias, bAlias} {
		p := map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias}
		code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, p)
		if err != nil || code != 201 {
			t.Fatalf("create account %s: code=%d err=%v body=%s", alias, code, err, string(body))
		}
		var acc struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &acc)
	}

	// Create balance trackers for both accounts
	trackerA, err := h.NewOperationTracker(ctx, trans, org.ID, ledger.ID, aAlias, "USD", headers)
	if err != nil {
		t.Fatalf("create tracker A: %v", err)
	}
	trackerB, err := h.NewOperationTracker(ctx, trans, org.ID, ledger.ID, bAlias, "USD", headers)
	if err != nil {
		t.Fatalf("create tracker B: %v", err)
	}

	// Seed A with 500
	seed := func(alias, amt string) {
		p := map[string]any{
			"code": isolation.UniqueTransactionCode("SEED"),
			"send": map[string]any{"asset": "USD", "value": amt, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": amt}}}}},
		}
		c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
		if e != nil || c != 201 {
			t.Fatalf("seed inflow %s: code=%d err=%v body=%s", alias, c, e, string(b))
		}
	}
	seed(aAlias, "500.00")

	// Wait for A to increase by 500
	if _, err := trackerA.VerifyDelta(ctx, decimal.RequireFromString("500.00"), 10*time.Second); err != nil {
		t.Fatalf("wait seed A: %v", err)
	}

	// Define operations
	jsonTransfer := func(fromAlias, toAlias, val string) (int, []byte, error) {
		p := map[string]any{
			"code": isolation.UniqueTransactionCode("TRF"),
			"send": map[string]any{
				"asset": "USD", "value": val,
				"source":     map[string]any{"from": []map[string]any{{"accountAlias": fromAlias, "amount": map[string]any{"asset": "USD", "value": val}}}},
				"distribute": map[string]any{"to": []map[string]any{{"accountAlias": toAlias, "amount": map[string]any{"asset": "USD", "value": val}}}},
			},
		}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID), headers, p)
	}
	outflow := func(alias, val string) (int, []byte, error) {
		p := map[string]any{
			"code": isolation.UniqueTransactionCode("OUT"),
			"send": map[string]any{"asset": "USD", "value": val, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
	}
	inflow := func(alias, val string) (int, []byte, error) {
		p := map[string]any{
			"code": isolation.UniqueTransactionCode("INF"),
			"send": map[string]any{"asset": "USD", "value": val, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
	}

	// Launch burst: 60 transfers A->B of 1.00 (<= seed), 20 outflows of 5.00, 30 inflows of 1.00
	var wg sync.WaitGroup
	trSucc, outSucc, inSucc := 0, 0, 0
	mu := sync.Mutex{}

	// Log failures for debugging
	var failures []string

	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, b, e := jsonTransfer(aAlias, bAlias, "1.00")
			if c == 201 {
				mu.Lock()
				trSucc++
				mu.Unlock()
			} else if e != nil || c != 201 {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("transfer[%d]: code=%d err=%v body=%s", idx, c, e, string(b)))
				mu.Unlock()
			}
		}(i)
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, b, e := outflow(aAlias, "5.00")
			if c == 201 {
				mu.Lock()
				outSucc++
				mu.Unlock()
			} else if e != nil || c != 201 {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("outflow[%d]: code=%d err=%v body=%s", idx, c, e, string(b)))
				mu.Unlock()
			}
		}(i)
	}
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, b, e := inflow(aAlias, "1.00")
			if c == 201 {
				mu.Lock()
				inSucc++
				mu.Unlock()
			} else if e != nil || c != 201 {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("inflow[%d]: code=%d err=%v body=%s", idx, c, e, string(b)))
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// Log first few failures if any
	if len(failures) > 0 {
		maxShow := 5
		if len(failures) < maxShow {
			maxShow = len(failures)
		}
		t.Logf("Transaction failures (showing first %d of %d): %v", maxShow, len(failures), failures[:maxShow])
	}

	// Calculate expected deltas from initial state
	// Delta A = initial seed (500) - transfers - outflows + inflows
	expDeltaA := decimal.RequireFromString("500").
		Sub(decimal.NewFromInt(int64(trSucc))).
		Sub(decimal.NewFromInt(int64(outSucc * 5))).
		Add(decimal.NewFromInt(int64(inSucc)))

	// Delta B = transfers received
	expDeltaB := decimal.NewFromInt(int64(trSucc))

	// Verify A's balance changed by expected delta
	gotA, err := trackerA.VerifyDelta(ctx, expDeltaA, 40*time.Second)
	if err != nil {
		actualDeltaA, _ := trackerA.GetCurrentDelta(ctx)
		t.Fatalf("A delta mismatch: actual_delta=%s expected_delta=%s err=%v (tr=%d out=%d in=%d)",
			actualDeltaA.String(), expDeltaA.String(), err, trSucc, outSucc, inSucc)
	}
	if gotA.IsNegative() {
		t.Fatalf("A negative final balance: %s", gotA.String())
	}

	// Verify B's balance changed by expected delta
	gotB, err := trackerB.VerifyDelta(ctx, expDeltaB, 40*time.Second)
	if err != nil {
		actualDeltaB, _ := trackerB.GetCurrentDelta(ctx)
		t.Fatalf("B delta mismatch: actual_delta=%s expected_delta=%s err=%v (tr=%d)",
			actualDeltaB.String(), expDeltaB.String(), err, trSucc)
	}
	if gotB.IsNegative() {
		t.Fatalf("B negative final balance: %s", gotB.String())
	}
}
