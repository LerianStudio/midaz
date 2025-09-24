package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "os"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Page 1 stability for ledgers within an org under concurrent creation.
func TestIntegration_Ledgers_PageStability_ConcurrentCreates(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_HEAVY") != "true" && os.Getenv("MIDAZ_TEST_NIGHTLY") != "true" {
        t.Skip("heavy test; set MIDAZ_TEST_HEAVY=true to run")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create org
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // Seed 6 ledgers
    for i := 0; i < 6; i++ {
        payload := h.LedgerPayloadRandom()
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, payload)
        if e != nil || c != 201 { t.Fatalf("create ledger %d: %d %s", i, c, string(b)) }
    }

    fetch := func(page int, limit int) []string {
        p := fmt.Sprintf("/v1/organizations/%s/ledgers?limit=%d&page=%d", org.ID, limit, page)
        c, b, e := onboard.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list ledgers: %d %s", c, string(b)) }
        var out struct{ Items []struct{ ID string `json:"id"` } `json:"items"` }
        _ = json.Unmarshal(b, &out)
        ids := make([]string, 0, len(out.Items))
        for _, it := range out.Items { ids = append(ids, it.ID) }
        return ids
    }

    p1 := fetch(1, 5)
    if len(p1) == 0 { t.Fatalf("empty p1") }

    // Create 3 more ledgers
    for i := 0; i < 3; i++ { _, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom()) }

    p1b := fetch(1, 5)
    if len(p1) != len(p1b) { t.Fatalf("p1 size changed: %d vs %d", len(p1), len(p1b)) }
    for i := range p1 { if p1[i] != p1b[i] { t.Fatalf("p1 changed at %d: %s vs %s", i, p1[i], p1b[i]) } }
}
