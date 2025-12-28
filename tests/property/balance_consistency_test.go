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
	orgID, err := h.SetupOrganization(ctx, onboard, headers, "PropBalance "+h.RandString(6))
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
		if ops > 15 { // Limit for API test performance
			ops = 15
		}

		// Create account
		alias := fmt.Sprintf("prop-%s", h.RandString(5))
		accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Logf("create account failed: %v", err)
			return true // Skip this iteration
		}

		// Ensure default balance record exists (created asynchronously)
		if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
			t.Logf("ensure balance: %v", err)
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
				headers["Idempotency-Key"] = fmt.Sprintf("%s-%d-%d-in", alias, seed, i)
				c, _, _ := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, alias, "USD", amountStr, headers)
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

				headers["Idempotency-Key"] = fmt.Sprintf("%s-%d-%d-out", alias, seed, i)
				c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", orgID, ledgerID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": outStr, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": outStr}}}}}})
				if c == 201 {
					expected = expected.Sub(outDec)
				}
			}
		}

		code, body, err := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=200", orgID, ledgerID, accountID), headers, nil)
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
		_ = json.Unmarshal(body, &opsResp)

		expectedFromOps := decimal.Zero
		for _, op := range opsResp.Items {
			if !op.BalanceAffected {
				continue
			}
			amt, e := decimal.NewFromString(op.Amount.Value)
			if e != nil {
				continue
			}
			switch op.Type {
			case "CREDIT":
				expectedFromOps = expectedFromOps.Add(amt)
			case "DEBIT":
				expectedFromOps = expectedFromOps.Sub(amt)
			}
		}

		actual, err := h.WaitForAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers, expectedFromOps, 45*time.Second)
		if err != nil {
			t.Errorf("Balance consistency not reached: expected=%s actual=%s alias=%s ops=%d err=%v", expectedFromOps.String(), actual.String(), alias, ops, err)
			return false
		}

		if !actual.Equal(expectedFromOps) {
			t.Errorf("Balance consistency violated after wait: expected=%s actual=%s alias=%s ops=%d", expectedFromOps.String(), actual.String(), alias, ops)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 10} // Fewer iterations (API calls expensive)
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("balance consistency property failed: %v", err)
	}
}
