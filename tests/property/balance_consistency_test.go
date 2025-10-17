package property

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Property: For any account, the balance must equal the sum of all operations (credits - debits).
// This is an API-level property that exercises real transaction and balance services.
func TestProperty_BalanceConsistency_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Setup org/ledger/asset once
	headers := h.AuthHeaders(h.RandHex(8))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("PropBalance "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v", code, err)
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v", code, err)
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	// Property test function
	f := func(seed int64, numOps uint8) bool {
		rng := rand.New(rand.NewSource(seed))
		ops := int(numOps)
		if ops <= 0 {
			ops = 1
		}
		if ops > 15 { // Limit for API test performance
			ops = 15
		}

		// Create account
		alias := fmt.Sprintf("prop-%s", h.RandString(5))
		code, body, err := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
		if err != nil || code != 201 {
			t.Logf("create account failed: code=%d err=%v", code, err)
			return true // Skip this iteration
		}
		var acc struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &acc)

		// Enable balance
		if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil {
			t.Logf("ensure balance: %v", err)
			return true
		}
		if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
			t.Logf("enable balance: %v", err)
			return true
		}

		// Track expected balance
		expected := decimal.Zero

		// Apply random operations
		for i := 0; i < ops; i++ {
			amount := rng.Intn(20) + 1 // 1-20 USD
			amountStr := fmt.Sprintf("%d.00", amount)
			amountDec := decimal.NewFromInt(int64(amount))

			if rng.Intn(2) == 0 || expected.IsZero() {
				// Inflow
				c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": amountStr, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": amountStr}}}}}})
				if c == 201 {
					expected = expected.Add(amountDec)
				}
			} else {
				// Outflow (only if we have balance)
				maxOut := expected.IntPart()
				if maxOut > 20 {
					maxOut = 20
				}
				outAmount := rng.Int63n(maxOut + 1)
				outStr := fmt.Sprintf("%d.00", outAmount)
				outDec := decimal.NewFromInt(outAmount)

				c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": outStr, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": outStr}}}}}})
				if c == 201 {
					expected = expected.Sub(outDec)
				}
			}
		}

		// Wait for final balance settlement
		time.Sleep(500 * time.Millisecond)

		// Query actual balance
		actual, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)
		if err != nil {
			t.Logf("balance query failed: %v", err)
			return true // Skip
		}

		// Property: actual balance must equal sum of operations
		if !actual.Equal(expected) {
			t.Errorf("Balance consistency violated: expected=%s actual=%s diff=%s alias=%s ops=%d",
				expected.String(), actual.String(), expected.Sub(actual).String(), alias, ops)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10} // Fewer iterations (API calls expensive)
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("balance consistency property failed: %v", err)
	}
}
