package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Reproduces 500 on revert after pending->commit flow, with diagnostic logs.
func TestDiagnostic_Revert500(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_DIAGNOSTIC") != "true" { t.Skip("diagnostic test; set MIDAZ_TEST_DIAGNOSTIC=true to run") }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Diag Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "diagR-"+h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed funds
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"10.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"10.00"}}}}}})

    // Create pending outflow, then commit
    p := map[string]any{"pending": true, "send": map[string]any{"asset":"USD","value":"3.00","source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"3.00"}}}}}}
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
    if err != nil || code != 201 { t.Fatalf("pending outflow: %d %s", code, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/commit", org.ID, ledger.ID, tx.ID), headers, nil)
    t.Logf("commit status=%d body=%s", code, string(body))

    // Revert (currently returns 500)
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/revert", org.ID, ledger.ID, tx.ID), headers, nil)
    t.Fatalf("revert status=%d body=%s err=%v", code, string(body), err)
}

