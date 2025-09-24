package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Combine metadata and date-range filters for accounts list.
func TestIntegration_Accounts_Filter_Metadata_And_DateRange(t *testing.T) {
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

    // create accounts with metadata tags today
    today := time.Now().Format("2006-01-02")
    mk := func(alias, dept string) {
        p := map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias,
            "metadata": map[string]any{"department": dept}}
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, p)
        if e != nil || c != 201 { t.Fatalf("create account: %d %s", c, string(b)) }
    }
    mk("acct-dept1-"+h.RandString(3), "sales")
    mk("acct-dept2-"+h.RandString(3), "ops")

    // filter by date range today and department=sales
    path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?start_date=%s&end_date=%s&metadata.department=sales&limit=50", org.ID, ledger.ID, today, today)
    code, body, err = onboard.Request(ctx, "GET", path, headers, nil)
    if err != nil || code != 200 { t.Fatalf("filter accounts: %d %s", code, string(body)) }
    var list struct{ Items []struct{ Metadata map[string]any `json:"metadata"` } `json:"items"` }
    _ = json.Unmarshal(body, &list)
    if len(list.Items) == 0 { t.Fatalf("expected at least one account for combined date+metadata filter") }
    for _, it := range list.Items {
        if it.Metadata["department"] != "sales" {
            t.Fatalf("unexpected account in combined date+metadata filter: %+v", it.Metadata)
        }
    }
}

