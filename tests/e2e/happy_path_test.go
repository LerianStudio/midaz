package e2e

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Happy-path E2E: org -> ledger -> portfolio + segment -> accounts -> transactions (inflow + JSON transfer) -> idempotency -> pagination -> numeric balances.
func TestE2E_HappyPathWorkflow(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Organization
    p := h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14))
    p["metadata"] = map[string]any{"tier":"gold"}
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, p)
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // Ledger
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("Ledger-%s", h.RandString(4))})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    // Portfolio
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/portfolios", org.ID, ledger.ID), headers, map[string]any{"name": "Main Portfolio"})
    if err != nil || code != 201 { t.Fatalf("create portfolio: code=%d err=%v body=%s", code, err, string(body)) }

    // Segment
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/segments", org.ID, ledger.ID), headers, map[string]any{"name": "Retail"})
    if err != nil || code != 201 { t.Fatalf("create segment: code=%d err=%v body=%s", code, err, string(body)) }

    // Accounts
    aliasA := fmt.Sprintf("a-%s", h.RandString(5)) // cash-like
    aliasB := fmt.Sprintf("b-%s", h.RandString(5)) // expense/revenue-like
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"Cash", "assetCode":"USD", "type":"deposit", "alias": aliasA})
    if err != nil || code != 201 { t.Fatalf("create account A: code=%d err=%v body=%s", code, err, string(body)) }
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"Expense", "assetCode":"USD", "type":"deposit", "alias": aliasB})
    if err != nil || code != 201 { t.Fatalf("create account B: code=%d err=%v body=%s", code, err, string(body)) }

    // Inflow to A: +25.50
    inflow := map[string]any{"send": map[string]any{"asset":"USD","value":"25.50","distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset":"USD","value":"25.50"}}}}}}
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, inflow)
    if err != nil || code != 201 { t.Fatalf("inflow: code=%d err=%v body=%s", code, err, string(body)) }

    // JSON transfer A -> B : 5.25
    jsonTxn := map[string]any{
        "send": map[string]any{
            "asset": "USD",
            "value": "5.25",
            "source": map[string]any{ "from": []map[string]any{ {"accountAlias": aliasA, "amount": map[string]any{"asset":"USD","value":"5.25"}} } },
            "distribute": map[string]any{ "to": []map[string]any{ {"accountAlias": aliasB, "amount": map[string]any{"asset":"USD","value":"5.25"}} } },
        },
    }
    idem := h.AuthHeaders(h.RandHex(8))
    idem["X-Idempotency"] = "i-" + h.RandHex(6)
    idem["X-TTL"] = "60"
    pathJSON := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID)
    code, body, err = trans.Request(ctx, "POST", pathJSON, idem, jsonTxn)
    if err != nil || code != 201 { t.Fatalf("json transfer: code=%d err=%v body=%s", code, err, string(body)) }
    // immediate replay to assert idempotency returns 201 with replay
    code, _, hdr, err := trans.RequestFull(ctx, "POST", pathJSON, idem, jsonTxn)
    if err != nil || code != 201 || hdr.Get("X-Idempotency-Replayed") == "" { t.Fatalf("expected idempotent replay, code=%d hdr=%s err=%v", code, hdr.Get("X-Idempotency-Replayed"), err) }

    // Balances for A: expect 25.50 - 5.25 = 20.25
    var pagedA struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
    code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, aliasA), headers, nil)
    if err != nil || code != 200 { t.Fatalf("balances A: code=%d err=%v body=%s", code, err, string(body)) }
    _ = json.Unmarshal(body, &pagedA)
    sumA := decimal.Zero
    for _, it := range pagedA.Items { if it.AssetCode == "USD" { sumA = sumA.Add(it.Available) } }
    wantA, _ := decimal.NewFromString("20.25")
    if !sumA.Equal(wantA) { t.Fatalf("account A balance want %s got %s", wantA, sumA) }

    // Balances for B: expect +5.25
    var pagedB struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
    code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, aliasB), headers, nil)
    if err != nil || code != 200 { t.Fatalf("balances B: code=%d err=%v body=%s", code, err, string(body)) }
    _ = json.Unmarshal(body, &pagedB)
    sumB := decimal.Zero
    for _, it := range pagedB.Items { if it.AssetCode == "USD" { sumB = sumB.Add(it.Available) } }
    wantB, _ := decimal.NewFromString("5.25")
    if !sumB.Equal(wantB) { t.Fatalf("account B balance want %s got %s", wantB, sumB) }

    // Accounts pagination across pages (limit=1) should list both aliases across pages without duplicates
    var list1, list2 struct{ Items []struct{ Alias *string } `json:"items"` }
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=1&page=1", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("accounts page1: code=%d err=%v body=%s", code, err, string(body)) }
    _ = json.Unmarshal(body, &list1)
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=1&page=2", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("accounts page2: code=%d err=%v body=%s", code, err, string(body)) }
    _ = json.Unmarshal(body, &list2)
    seen := map[string]bool{}
    for _, it := range list1.Items { if it.Alias != nil { seen[*it.Alias] = true } }
    for _, it := range list2.Items { if it.Alias != nil { if seen[*it.Alias] { t.Fatalf("duplicate alias across pages: %s", *it.Alias) } } }
}
