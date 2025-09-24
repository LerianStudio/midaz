package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "os"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Cursor stability during concurrent modifications: ensure page 1 (asc by created) remains stable after new ops appended.
func TestIntegration_Operations_CursorStability_ConcurrentAppends(t *testing.T) {
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
    alias := h.RandomAlias("cs")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed 8 operations (inflows)
    for i := 0; i < 8; i++ { _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "1.00", alias)) }

    type op struct{ ID string `json:"id"` }
    fetch := func(limit int) []op {
        p := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=%d&sort_order=asc", org.ID, ledger.ID, account.ID, limit)
        c, b, e := trans.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list ops: %d %s", c, string(b)) }
        var pr struct{ Items []op `json:"items"`; Pagination struct{ Items []op `json:"items"` } `json:"Pagination"` }
        _ = json.Unmarshal(b, &pr)
        if len(pr.Items) > 0 { return pr.Items }
        return pr.Pagination.Items
    }

    // Page 1 (limit=5, asc)
    p1 := fetch(5)
    if len(p1) == 0 { t.Fatalf("empty p1") }

    // Append 4 more operations
    for i := 0; i < 4; i++ { _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "1.00", alias)) }

    // Re-fetch page 1; expect it to be unchanged since new ops are later (asc ordering)
    p1b := fetch(5)
    if len(p1) != len(p1b) { t.Fatalf("p1 size changed: %d vs %d", len(p1), len(p1b)) }
    for i := range p1 { if p1[i].ID != p1b[i].ID { t.Fatalf("p1 content changed at %d: %s vs %s", i, p1[i].ID, p1b[i].ID) } }
}
