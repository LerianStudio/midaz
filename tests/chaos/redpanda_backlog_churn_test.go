// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Pause/unpause Redpanda amid transaction posts; API remains 2xx; final balances reflect successes.
//
//nolint:cyclop // chaos test covers multiple concurrent flows; splitting would obscure the test scenario
func TestChaos_Redpanda_BacklogChurn_AcceptsTransactions(t *testing.T) { //nolint:paralleltest // chaos tests interact with shared Docker infrastructure
	shouldRunChaos(t)

	cleanup := h.StartLogCapture([]string{"midaz-transaction", "midaz-onboarding", "midaz-redpanda"}, "Redpanda_BacklogChurn_AcceptsTransactions")
	defer cleanup()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account & seed 10
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Redpanda Backlog "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}

	var org struct {
		ID string `json:"id"`
	}
	mustUnmarshalJSON(t, body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-rb"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: %d %s", code, string(body))
	}

	var ledger struct {
		ID string `json:"id"`
	}
	mustUnmarshalJSON(t, body, &ledger)

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

	//nolint:dogsled // intentionally ignoring seed inflow result; success verified by WaitForAvailableSumByAlias
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "10.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "10.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("10.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Workers posting transactions for 5s
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

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

	// Pause Redpanda for ~2s, then unpause; continue traffic briefly
	if err := h.DockerAction("pause", "midaz-redpanda"); err == nil {
		time.Sleep(2 * time.Second)

		_ = h.DockerAction("unpause", "midaz-redpanda")
	}

	time.Sleep(3 * time.Second)
	close(stop)
	wg.Wait()

	// Verify final equals 10 + succ
	exp := decimal.NewFromInt(10).Add(decimal.NewFromInt(int64(succ)))
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, exp, 20*time.Second); err != nil {
		t.Fatalf("final wait after Redpanda backlog/churn: %v (succ=%d)", err, succ)
	}
}
