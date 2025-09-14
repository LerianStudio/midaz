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

// Pause/unpause RabbitMQ amid transaction posts; API remains 2xx; final balances reflect successes.
func TestChaos_RabbitMQ_BacklogChurn_AcceptsTransactions(t *testing.T) {
	shouldRunChaos(t)
	defer h.StartLogCapture([]string{"midaz-transaction", "midaz-onboarding", "midaz-rabbitmq"}, "RabbitMQ_BacklogChurn_AcceptsTransactions")()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account & seed 10
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("RMQ Backlog "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-rb"})
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
	alias := "rb-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: %d %s", code, string(body))
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "10.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "10.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("10.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Workers posting transactions for 5s
	var wg sync.WaitGroup
	var mu sync.Mutex
	succ := 0
	stop := make(chan struct{})
	worker := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			p := map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}}
			c, _, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
			if c == 201 {
				mu.Lock()
				succ++
				mu.Unlock()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wg.Add(2)
	go worker()
	go worker()

	// Pause RMQ for ~2s, then unpause; continue traffic briefly
	if err := h.DockerAction("pause", "midaz-rabbitmq"); err == nil {
		time.Sleep(2 * time.Second)
		_ = h.DockerAction("unpause", "midaz-rabbitmq")
	}
	time.Sleep(3 * time.Second)
	close(stop)
	wg.Wait()

	// Verify final equals 10 + succ
	exp := decimal.NewFromInt(10).Add(decimal.NewFromInt(int64(succ)))
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, exp, 20*time.Second); err != nil {
		t.Fatalf("final wait after RMQ backlog/churn: %v (succ=%d)", err, succ)
	}
}
