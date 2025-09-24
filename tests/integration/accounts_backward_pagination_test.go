package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Backward pagination consistency: fetch page 2 then page 1 again and ensure page 1 is unchanged.
func TestIntegration_Accounts_BackwardPagination_Consistency(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger + asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    // Create 6 accounts to ensure 3 pages with limit=2
    for i := 0; i < 6; i++ {
        alias := fmt.Sprintf("bp-%d-%s", i, h.RandString(3))
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
        if e != nil || c != 201 { t.Fatalf("create account %d: %d %s", i, c, string(b)) }
    }

    // fetch page 1 and page 2
    type acc struct{ ID string `json:"id"` }
    fetch := func(page int) []acc {
        c, b, e := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=2&page=%d", org.ID, ledger.ID, page), headers, nil)
        if e != nil || c != 200 { t.Fatalf("list page %d: %d %s", page, c, string(b)) }
        var out struct{ Items []acc `json:"items"` }
        _ = json.Unmarshal(b, &out)
        return out.Items
    }

    p1 := fetch(1)
    p2 := fetch(2)
    // navigate back to page 1 and compare
    p1b := fetch(1)
    if len(p1) != len(p1b) { t.Fatalf("page1 length mismatch: %d vs %d", len(p1), len(p1b)) }
    for i := range p1 { if p1[i].ID != p1b[i].ID { t.Fatalf("page1 order changed at %d: %s vs %s", i, p1[i].ID, p1b[i].ID) } }

    // sanity: page 2 items differ from page 1 items
    if len(p2) > 0 && len(p1) > 0 && p2[0].ID == p1[0].ID {
        t.Fatalf("unexpected overlap between page1 and page2 first items")
    }
}

