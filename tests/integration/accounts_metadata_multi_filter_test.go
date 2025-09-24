package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Multi-dimensional metadata filters for accounts (region + tier).
func TestIntegration_Accounts_Metadata_MultiFilter(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    // create accounts with metadata combinations
    mk := func(alias, region, tier string) {
        p := map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias,
            "metadata": map[string]any{"region": region, "tier": tier}}
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, p)
        if e != nil || c != 201 { t.Fatalf("create %s: %d %s", alias, c, string(b)) }
    }
    mk("acc-emea-gold-"+h.RandString(3), "emea", "gold")
    mk("acc-emea-silver-"+h.RandString(3), "emea", "silver")
    mk("acc-apac-gold-"+h.RandString(3), "apac", "gold")

    // filter by region=emea & tier=gold
    path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?metadata.region=emea&metadata.tier=gold&limit=50", org.ID, ledger.ID)
    code, body, err = onboard.Request(ctx, "GET", path, headers, nil)
    if err != nil || code != 200 { t.Fatalf("filter accounts: %d %s", code, string(body)) }
    var list struct{ Items []struct{ Metadata map[string]any `json:"metadata"` } `json:"items"` }
    if err := json.Unmarshal(body, &list); err != nil { t.Fatalf("unmarshal: %v", err) }
    if len(list.Items) == 0 { t.Fatalf("expected at least one account for combined filter") }
    for _, it := range list.Items {
        if it.Metadata["region"] != "emea" || it.Metadata["tier"] != "gold" {
            t.Fatalf("unexpected account in combined filter result: %+v", it.Metadata)
        }
    }
}

