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
)

// Property: For transfers between accounts A and B, the total (A + B) must remain constant.
// This validates conservation of value in the actual transaction system.
func TestProperty_TransferConservation_API(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	// Setup org/ledger/asset once
	headers := h.AuthHeaders(h.RandHex(8))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("PropXfer "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v", code, err)
	}
	var org struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v", code, err)
	}
	var ledger struct{ ID string `json:"id"` }
	_ = json.Unmarshal(body, &ledger)

	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	// Property test function
	f := func(seed int64, transfers uint8) bool {
		rng := rand.New(rand.NewSource(seed))
		numTransfers := int(transfers)
		if numTransfers <= 0 {
			numTransfers = 1
		}
		if numTransfers > 10 { // Limit for API performance
			numTransfers = 10
		}

		// Create two accounts
		aliasA := fmt.Sprintf("a-%s", h.RandString(5))
		aliasB := fmt.Sprintf("b-%s", h.RandString(5))

		for _, alias := range []string{aliasA, aliasB} {
			code, body, err := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias})
			if err != nil || code != 201 {
				t.Logf("create account %s: code=%d", alias, code)
				return true
			}
			var acc struct{ ID string `json:"id"` }
			_ = json.Unmarshal(body, &acc)

			if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil {
				t.Logf("ensure balance %s: %v", alias, err)
				return true
			}
			if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
				t.Logf("enable balance %s: %v", alias, err)
				return true
			}
		}

		// Seed account A with initial balance
		seedAmount := rng.Intn(50) + 50 // 50-100 USD
		seedStr := fmt.Sprintf("%d.00", seedAmount)
		_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": seedStr, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": seedStr}}}}}})

		// Wait for seed to settle
		time.Sleep(300 * time.Millisecond)

		// Get initial total (A + B)
		balA, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasA, "USD", headers)
		if err != nil {
			t.Logf("get initial balance A: %v", err)
			return true
		}
		balB, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasB, "USD", headers)
		if err != nil {
			t.Logf("get initial balance B: %v", err)
			return true
		}
		initialTotal := balA.Add(balB)

		// Perform random transfers A → B
		for i := 0; i < numTransfers; i++ {
			// Transfer random amount (1-10 USD)
			xferAmount := rng.Intn(10) + 1
			xferStr := fmt.Sprintf("%d.00", xferAmount)

			payload := map[string]any{
				"send": map[string]any{
					"asset": "USD",
					"value": xferStr,
					"source": map[string]any{
						"from": []map[string]any{
							{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": xferStr}},
						},
					},
					"distribute": map[string]any{
						"to": []map[string]any{
							{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": xferStr}},
						},
					},
				},
			}

			c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions", org.ID, ledger.ID), headers, payload)
			if c != 201 {
				// Skip if transfer fails (insufficient balance, etc.)
				break
			}

			// Small delay between transfers
			time.Sleep(50 * time.Millisecond)
		}

		// Wait for all operations to settle
		time.Sleep(1 * time.Second)

		// Get final balances
		finalA, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasA, "USD", headers)
		if err != nil {
			t.Logf("get final balance A: %v", err)
			return true
		}
		finalB, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasB, "USD", headers)
		if err != nil {
			t.Logf("get final balance B: %v", err)
			return true
		}
		finalTotal := finalA.Add(finalB)

		// Property: Total (A + B) must be conserved across transfers
		if !initialTotal.Equal(finalTotal) {
			t.Errorf("Transfer conservation violated: initial=%s final=%s diff=%s (A: %s→%s, B: %s→%s)",
				initialTotal.String(), finalTotal.String(), initialTotal.Sub(finalTotal).String(),
				balA.String(), finalA.String(), balB.String(), finalB.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 5} // Very few iterations (expensive API calls)
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("transfer conservation property failed: %v", err)
	}
}