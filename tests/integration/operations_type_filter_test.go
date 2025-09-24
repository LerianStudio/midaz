package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Ensures type filtering returns only DEBIT or CREDIT items as requested.
func TestIntegration_Operations_TypeFiltering(t *testing.T) {
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
    alias := h.RandomAlias("ty")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed both CREDIT (inflow) and DEBIT (outflow)
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "7.00", alias))
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, h.OutflowPayload(false, "USD", "2.00", alias))

    type op struct { Type string `json:"type"` }
    list := func(qtype string) []op {
        p := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?type=%s&limit=50", org.ID, ledger.ID, account.ID, qtype)
        c, b, e := trans.Request(ctx, "GET", p, headers, nil)
        if e != nil || c != 200 { t.Fatalf("list %s: %d %s", qtype, c, string(b)) }
        var out struct{ Items []op `json:"items"`; Pagination struct{ Items []op `json:"items"` } `json:"Pagination"` }
        _ = json.Unmarshal(b, &out)
        if len(out.Items) > 0 { return out.Items }
        return out.Pagination.Items
    }

    debits := list("DEBIT")
    if len(debits) == 0 { t.Fatalf("expected at least one DEBIT operation") }
    for _, it := range debits { if it.Type != "DEBIT" { t.Fatalf("unexpected type in DEBIT list: %s", it.Type) } }

    credits := list("CREDIT")
    if len(credits) == 0 { t.Fatalf("expected at least one CREDIT operation") }
    for _, it := range credits { if it.Type != "CREDIT" { t.Fatalf("unexpected type in CREDIT list: %s", it.Type) } }
}

