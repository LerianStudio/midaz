package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Read Replica Awareness: ensure read paths succeed when a replica is present.
// This test does not manipulate containers; it simply verifies common GET endpoints
// behave correctly with the default stack (primary + replica running).
func TestIntegration_ReadReplicaAwareness_ReadsSucceedWithReplica(t *testing.T) {
	env := h.LoadEnvironment()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))
	ctx := context.Background()

	// Create org
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Replica Org "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// Create ledger
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-rep"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	// Create USD asset and account
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := "rep-" + h.RandString(5)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}

	// Seed funds so reads have substance
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers,
		map[string]any{"send": map[string]any{"asset": "USD", "value": "42.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "42.00"}}}}}},
	)
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("42.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait for available=42.00: %v", err)
	}

	// Read paths under replica presence
	// 1) GET organization by ID
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get org by id: code=%d err=%v body=%s", code, err, string(body))
	}

	// 2) List ledgers under org
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list ledgers: code=%d err=%v body=%s", code, err, string(body))
	}

	// 3) List accounts under ledger
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list accounts: code=%d err=%v body=%s", code, err, string(body))
	}

	// 4) GET balances by alias
	code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get balances by alias: code=%d err=%v body=%s", code, err, string(body))
	}

	// 5) Sanity re-check available sum via helper
	if got, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers); err != nil {
		t.Fatalf("read available sum: %v", err)
	} else if !got.Equal(decimal.RequireFromString("42.00")) {
		t.Fatalf("unexpected available sum: got=%s want=42.00", got.String())
	}

	// 6) Stability sampling: perform consecutive reads and assert 200 and stable available sum
	for i := 0; i < 20; i++ {
		// list ledgers (general read path)
		code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, nil)
		if err != nil || code != 200 {
			t.Fatalf("iteration %d: list ledgers: code=%d err=%v body=%s", i, code, err, string(body))
		}
		// balances by alias remains 42.00
		if cur, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers); err != nil {
			t.Fatalf("iteration %d: get available sum err: %v", i, err)
		} else if !cur.Equal(decimal.RequireFromString("42.00")) {
			t.Fatalf("iteration %d: available sum drifted: got=%s want=42.00", i, cur.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
