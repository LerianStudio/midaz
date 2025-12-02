package fuzzy

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
	"github.com/shopspring/decimal"
)

func TestFuzz_Protocol_RapidFireAndRetries(t *testing.T) {
	shouldRun(t)
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Proto Org "+h.RandString(6), h.RandString(12)))
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
	alias := "proto-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: %d %s", code, string(body))
	}

	// Ensure default balance exists and is enabled
	var acc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acc)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil {
		t.Fatalf("ensure default balance ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default balance: %v", err)
	}

	// Seed balance
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "100.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "100.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("100.00"), 5*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Rapid-fire 50 mixed inflow/outflows with tiny random delays
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 50; i++ {
		val := fmt.Sprintf("%d.00", rng.Intn(3)+1) // 1,2,3
		if rng.Intn(2) == 0 {
			_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": val, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}})
		} else {
			_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": val, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}})
		}
		time.Sleep(time.Duration(rng.Intn(20)) * time.Millisecond)
	}

	// Idempotent retries: repeat same inflow request 5 times; expect either replay (201 + header) or one 201 + conflicts
	idemHeaders := h.AuthHeaders(h.RandHex(8))
	idemHeaders["X-Idempotency"] = "i-" + h.RandHex(6)
	idemHeaders["X-TTL"] = "60"
	inflow := map[string]any{"send": map[string]any{"asset": "USD", "value": "4.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "4.00"}}}}}}
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
	for j := 0; j < 5; j++ {
		code, _, hdr, err := trans.RequestFull(ctx, "POST", path, idemHeaders, inflow)
		if err != nil {
			t.Fatalf("idempotent inflow err: %v", err)
		}
		if !(code == 201 || code == 409) {
			t.Fatalf("unexpected code on retry %d: %d", j, code)
		}
		_ = hdr
	}
}
