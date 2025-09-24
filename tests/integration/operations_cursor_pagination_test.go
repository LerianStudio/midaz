package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "os"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Validates cursor-based pagination for account operations: forward with next_cursor and backward with prev_cursor.
func TestIntegration_Operations_CursorPagination_ForwardBackward(t *testing.T) {
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
    alias := h.RandomAlias("ops")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed 13 inflows -> expect at least 13 operations for the account
    for i := 0; i < 13; i++ {
        payload := h.InflowPayload("USD", "1.00", alias)
        c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, payload)
        if e != nil || c != 201 { t.Fatalf("inflow %d: %d %s", i, c, string(b)) }
    }

    // Helper to fetch a page by cursor
    type opItem struct { ID string `json:"id"` }
    type pageResp struct {
        Pagination struct {
            NextCursor string   `json:"next_cursor"`
            PrevCursor string   `json:"prev_cursor"`
            Items      []opItem `json:"items"`
            Limit      int      `json:"limit"`
        } `json:"Pagination"`
        // some builds return items/next_cursor at top-level too
        Items      []opItem `json:"items"`
        NextCursor string   `json:"next_cursor"`
        PrevCursor string   `json:"prev_cursor"`
        Limit      int      `json:"limit"`
    }
    fetch := func(cursor string, limit int) (items []opItem, next, prev string) {
        path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=%d", org.ID, ledger.ID, account.ID, limit)
        if cursor != "" { path += "&cursor=" + cursor }
        c, b, e := trans.Request(ctx, "GET", path, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list ops cursor: %d %s", c, string(b)) }
        var pr pageResp
        _ = json.Unmarshal(b, &pr)
        // prefer nested Pagination
        if len(pr.Pagination.Items) > 0 || pr.Pagination.NextCursor != "" || pr.Pagination.PrevCursor != "" {
            return pr.Pagination.Items, pr.Pagination.NextCursor, pr.Pagination.PrevCursor
        }
        return pr.Items, pr.NextCursor, pr.PrevCursor
    }

    // Forward: page1, page2, page3 (limit=5 => 5+5+3)
    p1, next1, _ := fetch("", 5)
    if len(p1) == 0 { t.Fatalf("empty page1 items") }
    p2, next2, _ := fetch(next1, 5)
    if len(p2) == 0 { t.Fatalf("empty page2 items") }
    p3, next3, prev3 := fetch(next2, 5)
    if next3 != "" && len(p3) != 5 { /* optional further pages */ }

    // Union and duplicate check
    seen := map[string]bool{}
    for _, it := range append(append([]opItem{}, p1...), append(p2, p3...)...) {
        if seen[it.ID] { t.Fatalf("duplicate operation across pages: %s", it.ID) }
        seen[it.ID] = true
    }
    if len(seen) < 13 { t.Fatalf("expected at least 13 operations, got %d", len(seen)) }

    // Backward: use prev3 to go back to page2 and compare items
    if prev3 != "" {
        p2b, _, _ := fetch(prev3, 5)
        if len(p2b) != len(p2) { t.Fatalf("backward page size mismatch: %d vs %d", len(p2b), len(p2)) }
        for i := range p2 {
            if p2[i].ID != p2b[i].ID { t.Fatalf("backward page mismatch at %d: %s vs %s", i, p2[i].ID, p2b[i].ID) }
        }
    }
}
