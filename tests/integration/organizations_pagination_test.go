package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_Organizations_ListPagination(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // create 3 orgs
    ids := make([]string, 0, 3)
    for i := 0; i < 3; i++ {
        code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org-%d-%s", i, h.RandString(3)), h.RandString(14)))
        if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
        var org struct{ ID string `json:"id"` }
        _ = json.Unmarshal(body, &org)
        ids = append(ids, org.ID)
    }

    // page 1 limit 2
    code, body, err := onboard.Request(ctx, "GET", "/v1/organizations?limit=2&page=1", headers, nil)
    if err != nil || code != 200 { t.Fatalf("list page1: code=%d err=%v body=%s", code, err, string(body)) }
    var p1 struct{ Items []struct{ ID string } `json:"items"` }
    _ = json.Unmarshal(body, &p1)
    if len(p1.Items) == 0 || len(p1.Items) > 2 { t.Fatalf("expected 1..2 items on page1, got %d", len(p1.Items)) }

    // page 2 limit 2
    code, body, err = onboard.Request(ctx, "GET", "/v1/organizations?limit=2&page=2", headers, nil)
    if err != nil || code != 200 { t.Fatalf("list page2: code=%d err=%v body=%s", code, err, string(body)) }
    var p2 struct{ Items []struct{ ID string } `json:"items"` }
    _ = json.Unmarshal(body, &p2)
    // ensure we don't see duplicates across page1 and page2
    seen := map[string]bool{}
    for _, it := range p1.Items { seen[it.ID] = true }
    for _, it := range p2.Items { if seen[it.ID] { t.Fatalf("duplicate org id across pages: %s", it.ID) } }
}
