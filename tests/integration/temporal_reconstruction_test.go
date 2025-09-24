package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Reconstruct available from operations list and compare with balances after a known sequence of ops.
func TestIntegration_Temporal_PointInTime_ReconstructionFromOperations(t *testing.T) {
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
    alias := h.RandomAlias("tt")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Sequence: +10.00, -3.00, +7.00
    // Using tiny sleeps to ensure distinct createdAt ordering
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "10.00", alias))
    time.Sleep(50 * time.Millisecond)
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, h.OutflowPayload(false, "USD", "3.00", alias))
    time.Sleep(50 * time.Millisecond)
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "7.00", alias))

    // Fetch operations and reconstruct: credits - debits (amount.value is in smallest unit; assume 2 decimals for USD)
    type op struct {
        Type      string `json:"type"`
        AssetCode string `json:"assetCode"`
        Amount    struct{ Value decimal.Decimal `json:"value"` } `json:"amount"`
        CreatedAt string `json:"createdAt"`
    }
    c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=50&sort_order=asc", org.ID, ledger.ID, account.ID), headers, nil)
    if e != nil || c != 200 { t.Fatalf("list ops: %d %s", c, string(b)) }
    var list struct{ Items []op `json:"items"`; Pagination struct{ Items []op `json:"items"` } `json:"Pagination"` }
    _ = json.Unmarshal(b, &list)
    items := list.Items
    if len(items) == 0 && len(list.Pagination.Items) > 0 { items = list.Pagination.Items }

    sum := decimal.Zero
    for _, it := range items {
        if it.AssetCode != "USD" { continue }
        amt := it.Amount.Value.Div(decimal.NewFromInt(100))
        switch it.Type {
        case "CREDIT":
            sum = sum.Add(amt)
        case "DEBIT":
            sum = sum.Sub(amt)
        }
    }

    // Compare with balances
    expected := decimal.RequireFromString("14.00")
    if !sum.Equal(expected) {
        t.Fatalf("reconstructed from operations=%s expected=%s", sum.String(), expected.String())
    }
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, 5*time.Second); err != nil {
        t.Fatalf("balances do not reflect reconstructed amount: %v", err)
    }
}

