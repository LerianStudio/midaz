package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Read-after-write within bounded time: new account appears in accounts list quickly.
func TestIntegration_EventualConsistency_AccountList_ReadAfterWrite(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger + asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    alias := h.RandomAlias("ra")
    // create account
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }

    // read-after-write: poll accounts?alias=<alias> (using list and filter client-side) within 3s
    deadline := time.Now().Add(3 * time.Second)
    for {
        c, b, e := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?limit=50&page=1", org.ID, ledger.ID), headers, nil)
        if e == nil && c == 200 {
            var list struct{ Items []struct{ Alias *string `json:"alias"` } `json:"items"` }
            _ = json.Unmarshal(b, &list)
            found := false
            for _, it := range list.Items { if it.Alias != nil && *it.Alias == alias { found = true; break } }
            if found { return }
        }
        if time.Now().After(deadline) { t.Fatalf("account not listed within bounded time") }
        time.Sleep(100 * time.Millisecond)
    }
}

// Operations list convergence: after an inflow, operations for account should include a CREDIT within bounded time.
func TestIntegration_EventualConsistency_Operations_ListAfterInflow(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // setup org/ledger/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("cv")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)

    // inflow
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "1.00", alias))

    // poll operations until we see at least one CREDIT
    deadline := time.Now().Add(5 * time.Second)
    for {
        c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=50", org.ID, ledger.ID, account.ID), headers, nil)
        if e == nil && c == 200 {
            var list struct{ Items []struct{ Type string `json:"type"` } `json:"items"`; Pagination struct{ Items []struct{ Type string `json:"type"` } `json:"items"` } `json:"Pagination"` }
            _ = json.Unmarshal(b, &list)
            items := list.Items
            if len(items) == 0 && len(list.Pagination.Items) > 0 { items = list.Pagination.Items }
            for _, it := range items { if it.Type == "CREDIT" { return } }
        }
        if time.Now().After(deadline) { t.Fatalf("operations did not converge within bounded time") }
        time.Sleep(100 * time.Millisecond)
    }
}

