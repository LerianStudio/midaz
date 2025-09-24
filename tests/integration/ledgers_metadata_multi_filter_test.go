package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Multi-dimensional metadata filters for ledgers (purpose + region) within an org.
func TestIntegration_Ledgers_Metadata_MultiFilter(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // ledgers with metadata
    mk := func(name, purpose, region string) {
        p := map[string]any{"name": name, "metadata": map[string]any{"purpose": purpose, "region": region}}
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, p)
        if e != nil || c != 201 { t.Fatalf("create ledger %s: %d %s", name, c, string(b)) }
    }
    mk("L-1", "ops", "emea")
    mk("L-2", "ops", "apac")
    mk("L-3", "finance", "emea")

    // filter by purpose=ops & region=emea
    path := fmt.Sprintf("/v1/organizations/%s/ledgers?metadata.purpose=ops&metadata.region=emea&limit=50", org.ID)
    code, body, err = onboard.Request(ctx, "GET", path, headers, nil)
    if err != nil || code != 200 { t.Fatalf("filter ledgers: %d %s", code, string(body)) }
    var list struct{ Items []struct{ Metadata map[string]any `json:"metadata"` } `json:"items"` }
    _ = json.Unmarshal(body, &list)
    if len(list.Items) == 0 { t.Fatalf("expected at least one ledger for combined filter") }
    for _, it := range list.Items {
        if it.Metadata["purpose"] != "ops" || it.Metadata["region"] != "emea" {
            t.Fatalf("unexpected ledger in combined filter result: %+v", it.Metadata)
        }
    }
}

