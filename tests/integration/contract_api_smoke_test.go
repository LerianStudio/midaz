package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Simple contract smoke: verify required fields exist and types look sane for Account GET and Transaction GET responses.
func TestContract_API_Smoke_AccountAndTransactionSchemas(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // setup org/ledger/asset/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "schema-" + h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)

    // GET account and check basic schema
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", org.ID, ledger.ID, account.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get account: %d %s", code, string(body)) }
    var acc map[string]any
    _ = json.Unmarshal(body, &acc)
    for _, k := range []string{"id","organizationId","ledgerId","assetCode","type"} {
        if _, ok := acc[k]; !ok { t.Fatalf("account missing required field %s", k) }
    }
    if _, ok := acc["metadata"]; !ok { t.Fatalf("account missing metadata object") }

    // create a small inflow and GET transaction by id
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "1.00", alias))
    if err != nil || code != 201 { t.Fatalf("create inflow: %d %s", code, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)
    code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", org.ID, ledger.ID, tx.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get transaction: %d %s", code, string(body)) }
    var tr map[string]any
    _ = json.Unmarshal(body, &tr)
    for _, k := range []string{"id","organizationId","ledgerId","status","operations"} {
        if _, ok := tr[k]; !ok { t.Fatalf("transaction missing field %s", k) }
    }
    if _, ok := tr["operations"].([]any); !ok { t.Fatalf("transaction.operations is not an array") }
}

