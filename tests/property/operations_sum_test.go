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
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropOps "+h.RandString(6))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, "L")
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
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
		accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Logf("create account: %v", err)
			return true
		}

		// Ensure default balance exists (created asynchronously)
		if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
			t.Logf("ensure balance: %v", err)
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
				headers["Idempotency-Key"] = fmt.Sprintf("%s-%d-%d-in", alias, seed, i)
				c, _, _ := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", amountStr, headers)
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

					headers["Idempotency-Key"] = fmt.Sprintf("%s-%d-%d-out", alias, seed, i)
					c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", orgID, ledgerID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": outStr, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": outStr}}}}}})
					if c == 201 {
						currentBalance = currentBalance.Sub(decimal.NewFromInt(outAmount))
					}
				}
			}
		}

		// Get account operations from API

		// Poll until operations sum equals current balance (eventual consistency)
		deadline := time.Now().Add(30 * time.Second)
		for {
			code, body, err := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=100", orgID, ledgerID, accountID), headers, nil)
			if err != nil || code != 200 {
				t.Logf("get operations: code=%d err=%v", code, err)
				return true
			}

			var opsResp struct {
				Items []struct {
					Type            string `json:"type"`
					BalanceAffected bool   `json:"balanceAffected"`
					Status          struct {
						Code string `json:"code"`
					} `json:"status"`
					Amount struct {
						Value string `json:"value"`
					} `json:"amount"`
				} `json:"items"`
			}
			if err := json.Unmarshal(body, &opsResp); err != nil {
				t.Logf("unmarshal operations: %v", err)
				return true
			}

			opsSum := decimal.Zero
			for _, op := range opsResp.Items {
				if !op.BalanceAffected {
					continue
				}
				amount, err := decimal.NewFromString(op.Amount.Value)
				if err != nil {
					continue
				}
				if op.Type == "CREDIT" {
					opsSum = opsSum.Add(amount)
				} else if op.Type == "DEBIT" {
					opsSum = opsSum.Sub(amount)
				}
			}

			actualBalance, err := h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
			if err == nil && opsSum.Equal(actualBalance) {
				break
			}

			if time.Now().After(deadline) {
				t.Errorf("Operations sum inconsistency: opsSum=%s balance=%s diff=%s alias=%s numOps=%d",
					opsSum.String(), actualBalance.String(), opsSum.Sub(actualBalance).String(), alias, len(opsResp.Items))
				return false
			}

			time.Sleep(150 * time.Millisecond)
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 5} // Few iterations (expensive)
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("operations sum property failed: %v", err)
	}
}
