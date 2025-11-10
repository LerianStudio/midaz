package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "sort"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Validates pagination invariants for accounts: union of pages equals full set; stable order; no duplicates.
func TestProperty_Pagination_UnionStableNoDuplicates(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Org + ledger + asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Prop Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    // Create 7 accounts with deterministic alias prefix
    aliases := make([]string, 0, 7)
    for i := 0; i < 7; i++ {
        alias := fmt.Sprintf("pp-%02d-%s", i, h.RandString(3))
        aliases = append(aliases, alias)
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
        if e != nil || c != 201 { t.Fatalf("create account %d: %d %s", i, c, string(b)) }
    }

    // List pages with limit=3 (expect 3 pages: 3 + 3 + 1)
    fetchPage := func(page int) []string {
        c, b, e := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=3&page=%d", org.ID, ledger.ID, page), headers, nil)
        if e != nil || c != 200 { t.Fatalf("list page %d: %d %s", page, c, string(b)) }
        var list struct{ Items []struct{ Alias *string `json:"alias"` } `json:"items"` }
        _ = json.Unmarshal(b, &list)
        outs := []string{}
        for _, it := range list.Items { if it.Alias != nil { outs = append(outs, *it.Alias) } }
        return outs
    }

    p1 := fetchPage(1)
    p2 := fetchPage(2)
    p3 := fetchPage(3)

    // Union equals full set (ignoring accounts from other tests) => at least the 7 we created must appear
    union := append(append([]string{}, p1...), append(p2, p3...)...)
    seen := map[string]int{}
    for _, a := range union { seen[a]++ }
    // Check no duplicates within union
    for a, cnt := range seen { if cnt > 1 { t.Fatalf("duplicate alias across pages: %s", a) } }
    // Ensure all created aliases are present
    for _, want := range aliases {
        if seen[want] == 0 { t.Fatalf("missing alias %s in paginated union", want) }
    }

    // Stable order: two consecutive reads of the first page should produce the same ordering
    p1b := fetchPage(1)
    if len(p1) != len(p1b) {
        t.Fatalf("page length changed between reads: %d vs %d", len(p1), len(p1b))
    }
    for i := range p1 {
        if p1[i] != p1b[i] { t.Fatalf("unstable order on page 1: %v vs %v", p1, p1b) }
    }

    // Optional: verify deterministic ordering by alias when sorted lexicographically (loose check)
    // NOTE: We don't enforce a specific order contract; this is a sanity only.
    tmp := append([]string{}, union...)
    sort.Strings(tmp)
    _ = tmp // placeholder sanity; not asserting hard sorting to avoid coupling.
}

