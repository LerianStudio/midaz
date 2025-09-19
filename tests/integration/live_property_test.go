package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Small, time-boxed live property: conservation of value under a sequence of inflows and bounded outflows.
func TestProperty_Live_Conservation_Small(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger + asset + account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }
    alias := fmt.Sprintf("prop-%s", h.RandString(5))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("ensure default balance ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default balance: %v", err) }

    expected := decimal.Zero
    steps := []decimal.Decimal{decimal.NewFromInt(2), decimal.NewFromInt(1), decimal.NewFromInt(3)} // 2, 1, 3 units

    // inflow 2
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value": steps[0].StringFixed(2), "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": steps[0].StringFixed(2)}}}}}})
    expected = expected.Add(steps[0])

    // outflow 1 (bounded)
    out := steps[1]
    if expected.GreaterThanOrEqual(out) {
        _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value": out.StringFixed(2), "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": out.StringFixed(2)}}}}}})
        expected = expected.Sub(out)
    }	

    // inflow 3
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value": steps[2].StringFixed(2), "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": steps[2].StringFixed(2)}}}}}})
    expected = expected.Add(steps[2])

    // Read balances and compare
    code, b, err := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get balances: code=%d err=%v body=%s", code, err, string(b)) }
    var paged struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
    _ = json.Unmarshal(b, &paged)
    sum := decimal.Zero
    for _, it := range paged.Items { if it.AssetCode == "USD" { sum = sum.Add(it.Available) } }
    if !sum.Equal(expected) {
        t.Fatalf("conservation live failed: want %s got %s", expected.String(), sum.String())
    }
}
