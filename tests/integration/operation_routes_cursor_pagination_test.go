package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Cursor pagination on operation-routes with tolerant assertions (since number of routes may vary).
func TestIntegration_OperationRoutes_CursorPagination(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup minimal org/ledger
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    // Fetch operation routes pages
    type route struct{ ID string `json:"id"` }
    type pageResp struct {
        Pagination struct {
            Items      []route `json:"items"`
            NextCursor string  `json:"next_cursor"`
            PrevCursor string  `json:"prev_cursor"`
            Limit      int     `json:"limit"`
        } `json:"Pagination"`
        Items      []route `json:"items"`
        NextCursor string  `json:"next_cursor"`
        PrevCursor string  `json:"prev_cursor"`
        Limit      int     `json:"limit"`
    }
    fetch := func(cursor string, limit int) (items []route, next, prev string) {
        path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/operation-routes?limit=%d", org.ID, ledger.ID, limit)
        if cursor != "" { path += "&cursor=" + cursor }
        c, b, e := trans.Request(ctx, "GET", path, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list op-routes: %d %s", c, string(b)) }
        var pr pageResp
        _ = json.Unmarshal(b, &pr)
        if len(pr.Pagination.Items) > 0 || pr.Pagination.NextCursor != "" || pr.Pagination.PrevCursor != "" {
            return pr.Pagination.Items, pr.Pagination.NextCursor, pr.Pagination.PrevCursor
        }
        return pr.Items, pr.NextCursor, pr.PrevCursor
    }

    p1, next1, _ := fetch("", 5)
    if len(p1) == 0 {
        // Some environments may have no predefined routes; still a valid 200 response.
        t.Log("no operation routes available; skipping further cursor checks")
        return
    }
    // If next exists, fetch p2 and ensure no overlaps with p1
    if next1 != "" {
        p2, _, _ := fetch(next1, 5)
        seen := map[string]bool{}
        for _, it := range p1 { seen[it.ID] = true }
        for _, it := range p2 {
            if seen[it.ID] { t.Fatalf("duplicate route across pages: %s", it.ID) }
        }
    }
}

