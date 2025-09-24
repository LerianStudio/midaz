package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Validate createdAt monotonicity (non-decreasing) for operations under sequential inserts.
func TestIntegration_Temporal_CreatedAt_Monotonicity(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // setup
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("tm")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)

    // insert 10 ops with tiny delays to encourage increasing timestamps
    for i := 0; i < 10; i++ {
        _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "1.00", alias))
        time.Sleep(10 * time.Millisecond)
    }

    // fetch asc and assert non-decreasing createdAt
    type op struct{ CreatedAt string `json:"createdAt"` }
    c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=50&sort_order=asc", org.ID, ledger.ID, account.ID), headers, nil)
    if e != nil || c != 200 { t.Fatalf("list ops: %d %s", c, string(b)) }
    var list struct{ Items []op `json:"items"`; Pagination struct{ Items []op `json:"items"` } `json:"Pagination"` }
    _ = json.Unmarshal(b, &list)
    items := list.Items
    if len(items) == 0 && len(list.Pagination.Items) > 0 { items = list.Pagination.Items }
    if len(items) < 10 { t.Fatalf("expected at least 10 ops, got %d", len(items)) }

    var prev time.Time
    for i, it := range items {
        t0, err := time.Parse(time.RFC3339, it.CreatedAt)
        if err != nil { t.Fatalf("parse createdAt at %d: %v (%s)", i, err, it.CreatedAt) }
        if i > 0 && t0.Before(prev) {
            t.Fatalf("createdAt not monotonic at %d: %s < %s", i, t0.String(), prev.String())
        }
        prev = t0
    }
}

