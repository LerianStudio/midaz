package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Validates date range filtering on operations list via start_date/end_date (YYYY-MM-DD).
func TestIntegration_Operations_DateRange_Filtering(t *testing.T) {
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
    alias := h.RandomAlias("dt")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed a few operations
    for i := 0; i < 3; i++ {
        payload := h.InflowPayload("USD", "1.00", alias)
        c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, payload)
        if e != nil || c != 201 { t.Fatalf("inflow %d: %d %s", i, c, string(b)) }
    }

    today := time.Now().Format("2006-01-02")
    tomorrow := time.Now().Add(24 * time.Hour).Format("2006-01-02")

    // Query with start_date today should include operations
    pathToday := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?start_date=%s&limit=50", org.ID, ledger.ID, account.ID, today)
    code, body, err = trans.Request(ctx, "GET", pathToday, headers, nil)
    if err != nil || code != 200 { t.Fatalf("ops today: %d %s err=%v", code, string(body), err) }
    var listToday struct{ Items []struct{ ID string `json:"id"` } `json:"items"`; Pagination struct{ Items []struct{ ID string `json:"id"` } `json:"items"` } `json:"Pagination"` }
    _ = json.Unmarshal(body, &listToday)
    items := listToday.Items
    if len(items) == 0 && len(listToday.Pagination.Items) > 0 { items = listToday.Pagination.Items }
    if len(items) < 3 { t.Fatalf("expected >=3 operations for today, got %d", len(items)) }

    // Query with start_date tomorrow should return zero for this account
    pathTomorrow := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations?start_date=%s&limit=50", org.ID, ledger.ID, account.ID, tomorrow)
    code, body, err = trans.Request(ctx, "GET", pathTomorrow, headers, nil)
    if err != nil || code != 200 { t.Fatalf("ops tomorrow: %d %s err=%v", code, string(body), err) }
    var listTomorrow struct{ Items []struct{ ID string `json:"id"` } `json:"items"`; Pagination struct{ Items []struct{ ID string `json:"id"` } `json:"items"` } `json:"Pagination"` }
    _ = json.Unmarshal(body, &listTomorrow)
    items2 := listTomorrow.Items
    if len(items2) == 0 && len(listTomorrow.Pagination.Items) > 0 { items2 = listTomorrow.Pagination.Items }
    if len(items2) != 0 {
        t.Fatalf("expected 0 operations for tomorrow start_date, got %d", len(items2))
    }
}

