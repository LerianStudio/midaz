package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Aggregation by metadata (client-side): seed operations categorized as sales and fees,
// then verify CREDIT sums grouped by metadata.category match seeded values.
func TestIntegration_Operations_Aggregation_ByMetadataCategory(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // setup org/ledger/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("agg")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }

    // seed categorized inflows
    post := func(value, category string) {
        p := h.InflowPayload("USD", value, alias)
        p["metadata"] = map[string]any{"category": category}
        c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
        if e != nil || c != 201 { t.Fatalf("inflow %s %s: %d %s", value, category, c, string(b)) }
    }
    post("10.00", "sales")
    post("15.00", "sales")
    post("3.00", "fees")

    // list operations and aggregate by metadata.category for CREDIT operations
    type op struct {
        Type     string                 `json:"type"`
        Amount   struct{ Value decimal.Decimal `json:"value"` } `json:"amount"`
        Metadata map[string]any         `json:"metadata"`
    }
    code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/operations?limit=100", org.ID, ledger.ID, alias), headers, nil)
    if err != nil || code != 200 { t.Fatalf("list ops: %d %s", code, string(body)) }
    var list struct{ Items []op `json:"items"`; Pagination struct{ Items []op `json:"items"` } `json:"Pagination"` }
    _ = json.Unmarshal(body, &list)
    items := list.Items
    if len(items) == 0 && len(list.Pagination.Items) > 0 { items = list.Pagination.Items }

    sums := map[string]decimal.Decimal{}
    for _, it := range items {
        if it.Type != "CREDIT" { continue }
        cat, _ := it.Metadata["category"].(string)
        amt := it.Amount.Value.Div(decimal.NewFromInt(100))
        s := sums[cat]
        sums[cat] = s.Add(amt)
    }
    if sums["sales"].Cmp(decimal.RequireFromString("25.00")) != 0 { t.Fatalf("sales sum mismatch: %s", sums["sales"]) }
    if sums["fees"].Cmp(decimal.RequireFromString("3.00")) != 0 { t.Fatalf("fees sum mismatch: %s", sums["fees"]) }
}

