package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Backward pagination consistency for ledgers within an org using metadata filter to isolate dataset.
func TestIntegration_Ledgers_BackwardPagination_Consistency(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create org
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    group := "bp-" + h.RandString(6)

    // Create 6 ledgers with metadata group
    for i := 0; i < 6; i++ {
        payload := h.LedgerPayloadRandom()
        payload["metadata"] = map[string]any{"bpgroup": group}
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, payload)
        if e != nil || c != 201 { t.Fatalf("create ledger %d: %d %s", i, c, string(b)) }
    }

    type ledgerItem struct{ ID string `json:"id"` }
    fetch := func(page int) []ledgerItem {
        p := fmt.Sprintf("/v1/organizations/%s/ledgers?limit=2&page=%d&metadata.bpgroup=%s", org.ID, page, group)
        c, b, e := onboard.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list ledgers page %d: %d %s", page, c, string(b)) }
        var out struct{ Items []ledgerItem `json:"items"` }
        _ = json.Unmarshal(b, &out)
        return out.Items
    }

    p1 := fetch(1)
    p2 := fetch(2)
    p1b := fetch(1)
    if len(p1) != len(p1b) { t.Fatalf("page1 size changed: %d vs %d", len(p1), len(p1b)) }
    for i := range p1 { if p1[i].ID != p1b[i].ID { t.Fatalf("page1 order changed at %d: %s vs %s", i, p1[i].ID, p1b[i].ID) } }
    if len(p2) > 0 && len(p1) > 0 && p2[0].ID == p1[0].ID { t.Fatalf("unexpected overlap between page1 and page2") }
}

