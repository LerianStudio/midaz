package integration

import (
    "context"
    "encoding/json"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Multi-dimensional metadata filters for organizations (region + tier).
func TestIntegration_Organizations_Metadata_MultiFilter(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    mk := func(region, tier string) string {
        p := h.OrgPayload("Org MM "+h.RandString(4), h.RandString(10))
        p["metadata"] = map[string]any{"region": region, "tier": tier}
        c, b, e := onboard.Request(ctx, "POST", "/v1/organizations", headers, p)
        if e != nil || c != 201 { t.Fatalf("create org: %d %s", c, string(b)) }
        var org struct{ ID string `json:"id"` }
        _ = json.Unmarshal(b, &org)
        return org.ID
    }
    _ = mk("emea", "gold")
    _ = mk("emea", "silver")
    _ = mk("apac", "gold")

    // filter: region=emea & tier=gold
    path := "/v1/organizations?metadata.region=emea&metadata.tier=gold&limit=50"
    code, body, err := onboard.Request(ctx, "GET", path, headers, nil)
    if err != nil || code != 200 { t.Fatalf("filter orgs: %d %s", code, string(body)) }
    var list struct{ Items []struct{ Metadata map[string]any `json:"metadata"` } `json:"items"` }
    _ = json.Unmarshal(body, &list)
    if len(list.Items) == 0 { t.Fatalf("expected at least one org for combined filter") }
    for _, it := range list.Items {
        if it.Metadata["region"] != "emea" || it.Metadata["tier"] != "gold" {
            t.Fatalf("unexpected org in combined filter result: %+v", it.Metadata)
        }
    }
}
