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
//nolint:cyclop,gocyclo,funlen // chaos test covers multiple concurrent flows + surface assertions; splitting would obscure the test scenario
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

	// Seed the account with a known 10 USD inflow. Assert the Request actually
	// succeeds rather than silently swallowing the error — otherwise this whole
	// test degenerates into "post nothing, observe nothing, pass". That was the
	// pre-fix behaviour flagged by the Batch B test-gap review.
	seedCode, seedBody, seedErr := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "10.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "10.00"}}}}}})
	if seedErr != nil {
		t.Fatalf("seed inflow request error: %v", seedErr)
	}

	if seedCode < 200 || seedCode >= 300 {
		t.Fatalf("seed inflow non-2xx: code=%d body=%s", seedCode, string(seedBody))
	}

	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("10.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Workers posting transactions for 5s
	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		transportErr error
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

			// Previously the worker discarded (code, body, err) unconditionally,
			// which made the "succ++ only on 201" counter the sole observable
			// and turned transport-level errors (connection reset during the
			// Redpanda pause) into silent no-ops. Propagate the err into the
			// mutex-protected tally so the final-balance assertion reflects
			// requests the service actually accepted, and log the first transport
			// error to surface real infrastructure problems during debugging.
			c, _, reqErr := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
			switch {
			case reqErr != nil:
				mu.Lock()
				if transportErr == nil {
					transportErr = reqErr
				}
				mu.Unlock()
			case c == 201:
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

	// If the transport failed (e.g. Redpanda pause also tore down the
	// transaction service briefly), surface the first error rather than
	// allowing a spuriously low succ count to make the balance assertion
	// trivially pass. This closes the "zero actions taken" hole.
	if succ == 0 {
		t.Fatalf("no transactions succeeded during Redpanda backlog/churn; transportErr=%v", transportErr)
	}

	if transportErr != nil {
		t.Logf("transport errors observed during chaos (succ=%d): %v", succ, transportErr)
	}

	// Verify final equals 10 + succ
	exp := decimal.NewFromInt(10).Add(decimal.NewFromInt(int64(succ)))
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, exp, 20*time.Second); err != nil {
		t.Fatalf("final wait after Redpanda backlog/churn: %v (succ=%d)", err, succ)
	}
}
