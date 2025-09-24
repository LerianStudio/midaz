package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Page 1 stability for accounts (page-based): after adding new accounts, page 1 should remain stable.
func TestIntegration_Accounts_PageStability_ConcurrentCreates(t *testing.T) {
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

    // Seed 6 accounts
    for i := 0; i < 6; i++ {
        alias := fmt.Sprintf("ps-%d-%s", i, h.RandString(3))
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
        if e != nil || c != 201 { t.Fatalf("create acc %d: %d %s", i, c, string(b)) }
    }

    fetch := func(page int, limit int) []string {
        p := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=%d&page=%d", org.ID, ledger.ID, limit, page)
        c, b, e := onboard.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list accounts: %d %s", c, string(b)) }
        var out struct{ Items []struct{ ID string `json:"id"` } `json:"items"` }
        _ = json.Unmarshal(b, &out)
        ids := make([]string, 0, len(out.Items))
        for _, it := range out.Items { ids = append(ids, it.ID) }
        return ids
    }

    p1 := fetch(1, 5)
    if len(p1) == 0 { t.Fatalf("empty page 1") }

    // Add 3 more accounts
    for i := 0; i < 3; i++ {
        alias := fmt.Sprintf("ps2-%d-%s", i, h.RandString(3))
        _, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    }

    p1b := fetch(1, 5)
    if len(p1) != len(p1b) { t.Fatalf("page1 size changed: %d vs %d", len(p1), len(p1b)) }
    for i := range p1 { if p1[i] != p1b[i] { t.Fatalf("page1 changed at %d: %s vs %s", i, p1[i], p1b[i]) } }
}

