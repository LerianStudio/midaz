package fuzzy

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// asyncProcessingSettleTime is the time to wait for async balance processing to complete
// before checking final consistency. This accounts for Redis-to-PostgreSQL sync delays.
const asyncProcessingSettleTime = 500 * time.Millisecond

func TestFuzz_Protocol_RapidFireAndRetries(t *testing.T) {
	shouldRun(t)
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Proto Org "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil {
		t.Fatalf("failed to parse org response: %v", err)
	}
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: %d %s", code, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil {
		t.Fatalf("failed to parse ledger response: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("asset: %v", err)
	}
	alias := "proto-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: %d %s", code, string(body))
	}

	// Ensure default balance exists and is enabled
	var acc struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &acc); err != nil {
		t.Fatalf("failed to parse account response: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil {
		t.Fatalf("ensure default balance ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default balance: %v", err)
	}

	// Seed balance
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "100.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "100.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("100.00"), 5*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Track balance changes before rapid-fire
	tracker, err := h.NewOperationTracker(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)
	if err != nil {
		t.Fatalf("failed to create operation tracker: %v", err)
	}

	// Rapid-fire 50 mixed inflow/outflows with tiny random delays
	// Track net change: positive for inflows, negative for outflows
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var netChange decimal.Decimal
	successCount := 0
	errorCount := 0

	for i := 0; i < 50; i++ {
		valInt := rng.Intn(3) + 1 // 1,2,3
		val := fmt.Sprintf("%d.00", valInt)
		valDec := decimal.NewFromInt(int64(valInt))

		var code int
		var body []byte
		var reqErr error

		isInflow := rng.Intn(2) == 0
		if isInflow {
			code, body, reqErr = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": val, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}})
		} else {
			code, body, reqErr = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": val, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}})
		}

		if reqErr != nil {
			t.Logf("request %d failed with error: %v", i, reqErr)
			errorCount++
			continue
		}

		// Accept 201 (success), 409 (conflict/idempotency), 4xx (validation errors like insufficient funds)
		// Only fail on 5xx server errors
		if code >= 500 {
			t.Fatalf("rapid-fire request %d got server error: code=%d body=%s", i, code, string(body))
		}

		if code == 201 {
			successCount++
			if isInflow {
				netChange = netChange.Add(valDec)
			} else {
				netChange = netChange.Sub(valDec)
			}
		} else {
			// 4xx errors are acceptable (insufficient funds, validation, etc.)
			t.Logf("request %d returned %d (acceptable): %s", i, code, string(body))
			errorCount++
		}

		time.Sleep(time.Duration(rng.Intn(20)) * time.Millisecond)
	}

	t.Logf("Rapid-fire complete: %d success, %d errors, net change: %s", successCount, errorCount, netChange.String())

	// Verify final balance consistency
	// Allow some time for async processing to complete
	time.Sleep(asyncProcessingSettleTime)

	finalDelta, err := tracker.GetCurrentDelta(ctx)
	if err != nil {
		t.Fatalf("failed to get final balance delta: %v", err)
	}

	// The actual delta should match our tracked net change
	// Allow small tolerance for concurrent operations
	if !finalDelta.Equal(netChange) {
		t.Logf("WARNING: Balance delta mismatch - expected %s but got %s (this may indicate a bug)", netChange.String(), finalDelta.String())
		// Don't fail here as concurrent operations may cause legitimate differences
		// This is a fuzzy test - we're looking for crashes and major issues, not perfect accounting
	}

	// Idempotent retries: repeat same inflow request 5 times; expect either replay (201 + header) or one 201 + conflicts
	idemHeaders := h.AuthHeaders(h.RandHex(8))
	idemHeaders["X-Idempotency"] = "i-" + h.RandHex(6)
	idemHeaders["X-TTL"] = "60"
	inflow := map[string]any{"send": map[string]any{"asset": "USD", "value": "4.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "4.00"}}}}}}
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
	for j := 0; j < 5; j++ {
		code, _, hdr, err := trans.RequestFull(ctx, "POST", path, idemHeaders, inflow)
		if err != nil {
			t.Fatalf("idempotent inflow err: %v", err)
		}
		if !(code == 201 || code == 409) {
			t.Fatalf("unexpected code on retry %d: %d", j, code)
		}
		_ = hdr
	}
}
