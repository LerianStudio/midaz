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

// Small, time-boxed live property: conservation of value under a sequence of inflows and bounded outflows.
func TestProperty_Live_Conservation_Small(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// org + ledger + asset + account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := fmt.Sprintf("prop-%s", h.RandString(5))
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var account struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &account)

	expected := decimal.Zero
	steps := []decimal.Decimal{decimal.NewFromInt(2), decimal.NewFromInt(1), decimal.NewFromInt(3)} // 2, 1, 3 units

	// inflow 2
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": steps[0].StringFixed(2), "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": steps[0].StringFixed(2)}}}}}})
	expected = expected.Add(steps[0])

	// outflow 1 (bounded)
	out := steps[1]
	if expected.GreaterThanOrEqual(out) {
		_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": out.StringFixed(2), "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": out.StringFixed(2)}}}}}})
		expected = expected.Sub(out)
	}

	// inflow 3
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": steps[2].StringFixed(2), "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": steps[2].StringFixed(2)}}}}}})
	expected = expected.Add(steps[2])

	// Wait until balances reflect the expected available sum, then compare
	timeout := 5 * time.Second
	if td, ok := t.Deadline(); ok {
		if d := time.Until(td) / 2; d < timeout {
			timeout = d
		}
	}
	sum, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, timeout)
	if err != nil {
		t.Fatalf("conservation live failed: %v (last=%s expected=%s)", err, sum.String(), expected.String())
	}
}
