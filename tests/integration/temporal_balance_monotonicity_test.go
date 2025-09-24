package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Balance monotonicity: consecutive inflows should produce non-decreasing available; consecutive outflows (<= available) should be non-increasing.
func TestIntegration_Temporal_BalanceMonotonicity_InflowThenOutflow(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // setup org/ledger/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("bm")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }

    // helper to get available
    getAvail := func() decimal.Decimal {
        c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias), headers, nil)
        if e != nil || c != 200 { t.Fatalf("get balances: %d %s", c, string(b)) }
        var paged struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
        _ = json.Unmarshal(b, &paged)
        sum := decimal.Zero
        for _, it := range paged.Items { if it.AssetCode == "USD" { sum = sum.Add(it.Available) } }
        return sum
    }

    // inflows
    inflows := []string{"1.00", "2.00", "3.00"}
    prev := decimal.Zero
    for _, v := range inflows {
        _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", v, alias))
        // small wait for availability to reflect
        _, _ = h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, prev.Add(decimal.RequireFromString(v)), 5*time.Second)
        cur := getAvail()
        if cur.LessThan(prev) { t.Fatalf("available decreased after inflow: %s -> %s", prev, cur) }
        prev = cur
    }

    // outflows: ensure amounts do not exceed available
    outs := []string{"1.00", "1.50"}
    for _, v := range outs {
        _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, h.OutflowPayload(false, "USD", v, alias))
        // compute expected fallback: not strictly required; just check monotonic non-increasing
        time.Sleep(50 * time.Millisecond)
        cur := getAvail()
        if cur.GreaterThan(prev) { t.Fatalf("available increased after outflow: %s -> %s", prev, cur) }
        prev = cur
    }
}

