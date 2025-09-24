package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Backward pagination consistency for organizations with metadata filter to isolate dataset.
func TestIntegration_Organizations_BackwardPagination_Consistency(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    group := "bp-" + h.RandString(6)

    // Create 6 organizations tagged with group
    for i := 0; i < 6; i++ {
        payload := h.OrgPayload("BP Org "+h.RandString(4), h.RandString(10))
        payload["metadata"] = map[string]any{"bpgroup": group}
        c, b, e := onboard.Request(ctx, "POST", "/v1/organizations", headers, payload)
        if e != nil || c != 201 { t.Fatalf("create org %d: %d %s", i, c, string(b)) }
    }

    // Helper fetch by page with metadata filter
    type org struct{ ID string `json:"id"` }
    fetch := func(page int) []org {
        p := fmt.Sprintf("/v1/organizations?limit=2&page=%d&metadata.bpgroup=%s", page, group)
        c, b, e := onboard.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list orgs page %d: %d %s", page, c, string(b)) }
        var out struct{ Items []org `json:"items"` }
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

