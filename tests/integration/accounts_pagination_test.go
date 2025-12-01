package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_Accounts_ListPagination(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }

    // create 3 accounts
    for i := 0; i < 3; i++ {
        alias := fmt.Sprintf("pacc-%d-%s", i, h.RandString(3))
        _, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    }

    // page 1 limit 2
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=2&page=1", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("accounts page1: code=%d err=%v body=%s", code, err, string(body)) }
    var p1 struct{ Items []struct{ ID string `json:"id"` } `json:"items"` }
    _ = json.Unmarshal(body, &p1)
    if len(p1.Items) == 0 || len(p1.Items) > 2 { t.Fatalf("expected 1..2 items on page1, got %d", len(p1.Items)) }

    // page 2 limit 2
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=2&page=2", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("accounts page2: code=%d err=%v body=%s", code, err, string(body)) }
    var p2 struct{ Items []struct{ ID string `json:"id"` } `json:"items"` }
    _ = json.Unmarshal(body, &p2)
    seen := map[string]bool{}
    for _, it := range p1.Items { seen[it.ID] = true }
    for _, it := range p2.Items { if seen[it.ID] { t.Fatalf("duplicate account id across pages: %s", it.ID) } }
}
