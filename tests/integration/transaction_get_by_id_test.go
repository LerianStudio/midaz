package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_Transaction_GetByID(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // setup org/ledger/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }
    alias := fmt.Sprintf("tget-%s", h.RandString(4))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("ensure default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // create inflow
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"2.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"2.00"}}}}}})
    if err != nil || code != 201 { t.Fatalf("create inflow: code=%d err=%v body=%s", code, err, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)

    // GET by ID
    code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", org.ID, ledger.ID, tx.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get transaction by id: code=%d err=%v body=%s", code, err, string(body)) }
    var txGet struct{ ID string `json:"id"`; OrganizationID string `json:"organizationId"`; LedgerID string `json:"ledgerId"`; Status struct{ Code string `json:"code"` } `json:"status"` }
    if err := json.Unmarshal(body, &txGet); err != nil { t.Fatalf("parse tx: %v", err) }
    if txGet.ID != tx.ID { t.Fatalf("transaction id mismatch: want=%s got=%s", tx.ID, txGet.ID) }
    if txGet.OrganizationID == "" || txGet.LedgerID == "" || txGet.Status.Code == "" {
        t.Fatalf("unexpected empty fields in transaction: %+v", txGet)
    }
}
