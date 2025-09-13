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

// Runs a smaller mixed burst and logs counts and final balance to highlight mismatch between applied ops and final state.
func TestDiagnostic_BurstConsistency(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/accounts
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Diag Org "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: %d %s", code, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("asset: %v", err)
	}

	alias := "diagC-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: %d %s", code, string(body))
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}

	// Seed 100
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "100.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "100.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("100.00"), 5*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Burst: 10 outflows of 5, 20 inflows of 2
	outflow := func(val string) (int, []byte, error) {
		p := map[string]any{"send": map[string]any{"asset": "USD", "value": val, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
	}
	inflow := func(val string) (int, []byte, error) {
		p := map[string]any{"send": map[string]any{"asset": "USD", "value": val, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
	}

	var wg sync.WaitGroup
	outSucc, inSucc := 0, 0
	mu := sync.Mutex{}
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, _ := inflow("2.00")
			if c == 201 {
				mu.Lock()
				inSucc++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	exp := decimal.RequireFromString("100").Sub(decimal.NewFromInt(int64(outSucc * 5))).Add(decimal.NewFromInt(int64(inSucc * 2)))
	got, _ := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)
	t.Logf("outSucc=%d inSucc=%d expected=%s got=%s", outSucc, inSucc, exp.String(), got.String())
	if !got.Equal(exp) {
		t.Fatalf("final mismatch")
	}
}
