package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
	"github.com/shopspring/decimal"
)

// Sequentially restart onboarding then transaction while idempotent writes continue; verify no duplicates and correct final.
func TestChaos_RollingRestarts_UnderLoad_Idempotent(t *testing.T) {
	shouldRunChaos(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account & seed 20
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Roll Org "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-roll"})
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
	alias := "roll-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: %d %s", code, string(body))
	}
	var acc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acc)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "20.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "20.00"}}}}}})

	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("20.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Writer with unique idempotency for each request
	var wg sync.WaitGroup
	var mu sync.Mutex
	succ := 0
	stop := make(chan struct{})
	writer := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			hdr := h.AuthHeaders(h.RandHex(6))
			hdr["X-Idempotency"] = "roll-" + h.RandHex(8)
			p := map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}}
			c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), hdr, p)
			if c == 201 {
				mu.Lock()
				succ++
				mu.Unlock()
			}
			time.Sleep(15 * time.Millisecond)
		}
	}
	wg.Add(1)
	go writer()

	// Restart onboarding then transaction sequentially
	_ = h.DockerAction("restart", "midaz-ledger")
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
	_ = h.DockerAction("restart", "midaz-ledger")
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)

	time.Sleep(3 * time.Second)
	close(stop)
	wg.Wait()

	exp := decimal.NewFromInt(20).Add(decimal.NewFromInt(int64(succ)))
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, exp, 30*time.Second); err != nil {
		t.Fatalf("final mismatch after rolling restarts: %v (succ=%d)", err, succ)
	}
}
