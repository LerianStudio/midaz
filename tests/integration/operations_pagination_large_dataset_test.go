package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "os"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Large dataset pagination: 120 operations with limit=100; verify union across pages without duplicates.
func TestIntegration_Operations_Pagination_LargeDataset(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_HEAVY") != "true" && os.Getenv("MIDAZ_TEST_NIGHTLY") != "true" {
        t.Skip("heavy test; set MIDAZ_TEST_HEAVY=true to run")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("ld")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed 120 operations
    for i := 0; i < 120; i++ { _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "1.00", alias)) }

    type op struct{ ID string `json:"id"` }
    type page struct { Items []op `json:"items"`; Pagination struct { Items []op `json:"items"`; NextCursor string `json:"next_cursor"` } `json:"Pagination"`; NextCursor string `json:"next_cursor"` }
    fetch := func(cursor string) (items []op, next string) {
        p := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=100", org.ID, ledger.ID, account.ID)
        if cursor != "" { p += "&cursor=" + cursor }
        c, b, e := trans.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list ops: %d %s", c, string(b)) }
        var out page
        _ = json.Unmarshal(b, &out)
        items = out.Items
        next = out.NextCursor
        if len(items) == 0 && len(out.Pagination.Items) > 0 { items = out.Pagination.Items; next = out.Pagination.NextCursor }
        return
    }

    p1, next := fetch("")
    if len(p1) == 0 || len(p1) > 100 { t.Fatalf("unexpected p1 size: %d", len(p1)) }
    p2, _ := fetch(next)
    seen := map[string]bool{}
    for _, it := range p1 { if seen[it.ID] { t.Fatalf("dup in p1: %s", it.ID) } ; seen[it.ID] = true }
    for _, it := range p2 {
        if seen[it.ID] { t.Fatalf("duplicate across pages: %s", it.ID) }
        seen[it.ID] = true
    }
    if len(seen) < 120 { t.Fatalf("expected >=120 operations across pages, got %d", len(seen)) }
}
