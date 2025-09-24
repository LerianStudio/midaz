package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "os"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Page 1 stability for organizations under concurrent creation with group metadata filter to isolate dataset.
func TestIntegration_Organizations_PageStability_ConcurrentCreates(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_HEAVY") != "true" && os.Getenv("MIDAZ_TEST_NIGHTLY") != "true" {
        t.Skip("heavy test; set MIDAZ_TEST_HEAVY=true to run")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    group := "psg-" + h.RandString(6)

    // Seed 6 orgs in this group
    for i := 0; i < 6; i++ {
        p := h.OrgPayload("PS Org "+h.RandString(4), h.RandString(10))
        p["metadata"] = map[string]any{"group": group}
        c, b, e := onboard.Request(ctx, "POST", "/v1/organizations", headers, p)
        if e != nil || c != 201 { t.Fatalf("create org %d: %d %s", i, c, string(b)) }
    }

    fetch := func(page int, limit int) []string {
        path := fmt.Sprintf("/v1/organizations?metadata.group=%s&limit=%d&page=%d", group, limit, page)
        c, b, e := onboard.Request(ctx, "GET", path, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list orgs: %d %s", c, string(b)) }
        var out struct{ Items []struct{ ID string `json:"id"` } `json:"items"` }
        _ = json.Unmarshal(b, &out)
        ids := make([]string, 0, len(out.Items))
        for _, it := range out.Items { ids = append(ids, it.ID) }
        return ids
    }

    p1 := fetch(1, 5)
    if len(p1) == 0 { t.Fatalf("empty p1") }

    // Add 3 more orgs in the group
    for i := 0; i < 3; i++ {
        p := h.OrgPayload("PS Org2 "+h.RandString(4), h.RandString(10))
        p["metadata"] = map[string]any{"group": group}
        _, _, _ = onboard.Request(ctx, "POST", "/v1/organizations", headers, p)
    }

    p1b := fetch(1, 5)
    if len(p1) != len(p1b) { t.Fatalf("p1 size changed: %d vs %d", len(p1), len(p1b)) }
    for i := range p1 { if p1[i] != p1b[i] { t.Fatalf("p1 changed at %d: %s vs %s", i, p1[i], p1b[i]) } }
}
