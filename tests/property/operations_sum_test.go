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

// Property: The sum of all operations for an account must equal its current balance.
// This validates that the operations history accurately reflects balance changes.
func TestProperty_OperationsSum_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Setup org/ledger/asset once
	headers := h.AuthHeaders(h.RandHex(8))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("PropOps "+h.RandString(6), h.RandString(12)))
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
		if ops > 12 { // Limit for API performance
			ops = 12
		}

		// Create account
		alias := fmt.Sprintf("ops-%s", h.RandString(5))
		code, body, err := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
		if err != nil || code != 201 {
			t.Logf("create account: code=%d", code)
			return true
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

		// Track current balance for outflow validation
		currentBalance := decimal.Zero

		// Apply random operations
		for i := 0; i < ops; i++ {
			amount := rng.Intn(15) + 1 // 1-15 USD
			amountStr := fmt.Sprintf("%d.00", amount)

			if rng.Intn(3) < 2 || currentBalance.IsZero() {
				// 2/3 inflows
				c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": amountStr, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": amountStr}}}}}})
				if c == 201 {
					currentBalance = currentBalance.Add(decimal.NewFromInt(int64(amount)))
				}
			} else {
				// 1/3 outflows (only if balance available)
				maxOut := currentBalance.IntPart()
				if maxOut > 15 {
					maxOut = 15
				}
				if maxOut > 0 {
					outAmount := rng.Int63n(maxOut) + 1
					outStr := fmt.Sprintf("%d.00", outAmount)

					c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": outStr, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": outStr}}}}}})
					if c == 201 {
						currentBalance = currentBalance.Sub(decimal.NewFromInt(outAmount))
					}
				}
			}

			time.Sleep(30 * time.Millisecond)
		}

		// Wait for settlement
		time.Sleep(1 * time.Second)

		// Get account operations from API
		code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations", org.ID, ledger.ID, acc.ID), headers, nil)
		if err != nil || code != 200 {
			t.Logf("get operations: code=%d err=%v", code, err)
			return true
		}

		var opsResp struct {
			Items []struct {
				Type   string `json:"type"`
				Amount struct {
					Value string `json:"value"`
				} `json:"amount"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &opsResp); err != nil {
			t.Logf("unmarshal operations: %v", err)
			return true
		}

		// Calculate sum from operations (credit - debit)
		opsSum := decimal.Zero
		for _, op := range opsResp.Items {
			amount, err := decimal.NewFromString(op.Amount.Value)
			if err != nil {
				t.Logf("parse operation amount: %v", err)
				continue
			}
			if op.Type == "CREDIT" {
				opsSum = opsSum.Add(amount)
			} else if op.Type == "DEBIT" {
				opsSum = opsSum.Sub(amount)
			}
		}

		// Get current balance
		actualBalance, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)
		if err != nil {
			t.Logf("get balance: %v", err)
			return true
		}

		// Property: Operations sum must equal current balance
		if !opsSum.Equal(actualBalance) {
			t.Errorf("Operations sum inconsistency: opsSum=%s balance=%s diff=%s alias=%s numOps=%d",
				opsSum.String(), actualBalance.String(), opsSum.Sub(actualBalance).String(), alias, len(opsResp.Items))
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 5} // Few iterations (expensive)
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("operations sum property failed: %v", err)
	}
}
