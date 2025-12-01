package chaos

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

// Soak test (guarded by MIDAZ_TEST_SOAK): mixed traffic with periodic chaos injections; verify invariants at end.
func TestChaos_Soak_MixedTraffic_WithInjections(t *testing.T) {
	shouldRunChaos(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/accounts A & B, seed A=100
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Soak Org "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-soak"})
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
	a, b := "skA-"+h.RandString(4), "skB-"+h.RandString(4)
	_, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": a})
	_, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "B", "assetCode": "USD", "type": "deposit", "alias": b})
	_ = h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, a, headers)
	_ = h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, b, headers)
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "100.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": a, "amount": map[string]any{"asset": "USD", "value": "100.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, a, "USD", headers, decimal.RequireFromString("100.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Mixed traffic goroutines
	var wg sync.WaitGroup
	stop := make(chan struct{})
	traffic := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// random pick among inflow/outflow/transfer with small amounts
			_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": a, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}})
			_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "source": map[string]any{"from": []map[string]any{{"accountAlias": a, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}})
			_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "source": map[string]any{"from": []map[string]any{{"accountAlias": a, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": b, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}})
			time.Sleep(20 * time.Millisecond)
		}
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go traffic()
	}

	// Periodic injections every minute for up to 5 minutes
	duration := 5 * time.Minute
	end := time.Now().Add(duration)
	for time.Now().Before(end) {
		_ = h.DockerAction("restart", "midaz-ledger")
		_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
		time.Sleep(60 * time.Second)
	}

	close(stop)
	wg.Wait()

	// Sanity: check balances are non-negative and finite
	if curA, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, a, "USD", headers); err != nil || curA.IsNegative() {
		t.Fatalf("A balance invalid after soak: %v curA=%s", err, curA.String())
	}
	if curB, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, b, "USD", headers); err != nil || curB.IsNegative() {
		t.Fatalf("B balance invalid after soak: %v curB=%s", err, curB.String())
	}
}
