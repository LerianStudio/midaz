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

// Hard kill vs graceful stop on services during active traffic; verify recovery and no data loss.
func TestChaos_HardKillVsStop_ServicesDuringTraffic(t *testing.T) {
	shouldRunChaos(t)
	defer h.StartLogCapture([]string{"midaz-ledger", "midaz-ledger", "midaz-postgres-primary"}, "HardKillVsStop_ServicesDuringTraffic")()

	env := h.LoadEnvironment()
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account and seed
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("KillStop Org "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-ks"})
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
	alias := "ks-" + h.RandString(4)
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
	// Seed 50
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "50.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "50.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("50.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Writers: inflows of 1.00 with per-request idempotency
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
			hdr["X-Idempotency"] = "idem-" + h.RandHex(8)
			payload := map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}}
			c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), hdr, payload)
			if c == 201 {
				mu.Lock()
				succ++
				mu.Unlock()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wg.Add(1)
	go writer()

	// Exercise kill and stop across services
	type step struct{ action, container string }
	steps := []step{
		{"kill", "midaz-ledger"},
		{"start", "midaz-ledger"},
		{"stop", "midaz-ledger"},
		{"start", "midaz-ledger"},
		{"kill", "midaz-ledger"},
		{"start", "midaz-ledger"},
	}
	for _, s := range steps {
		if err := h.DockerAction(s.action, s.container); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("docker %s %s: %v", s.action, s.container, err)
		}
		// Wait briefly and then health-check if it is a service
		if s.action == "start" {
			if s.container == "midaz-ledger" {
				_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
			}
			if s.container == "midaz-ledger" {
				_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
			}
		}
		time.Sleep(1 * time.Second)
	}

	// Also kill and start Postgres primary while writes continue
	if err := h.DockerAction("kill", "midaz-postgres-primary"); err == nil {
		// start again and wait a bit
		_ = h.DockerAction("start", "midaz-postgres-primary")
		time.Sleep(3 * time.Second)
	}

	// stop writers and reconcile
	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()

	// We assert no data loss: final must be >= initial + observed successes.
	minExpected := decimal.RequireFromString("50").Add(decimal.NewFromInt(int64(succ)))
	got, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)
	if err != nil {
		t.Fatalf("read final balance: %v", err)
	}
	if got.LessThan(minExpected) {
		t.Fatalf("possible data loss after kill/stop: got=%s minExpected=%s succ=%d", got.String(), minExpected.String(), succ)
	}
}
