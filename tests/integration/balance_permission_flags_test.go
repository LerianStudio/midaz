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

// Ensure allowSending flag on default balance prevents outflows when false and permits when true.
func TestIntegration_BalancePermissionFlags_BlockAndAllow(t *testing.T) {
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
    alias := h.RandomAlias("perm")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.SetDefaultBalanceFlags(ctx, trans, org.ID, ledger.ID, alias, headers, true, true); err != nil { t.Fatalf("init flags: %v", err) }

    // seed
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "5.00", alias))
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("5.00"), 5*time.Second); err != nil { t.Fatalf("seed wait: %v", err) }

    // disable sending
    if err := h.SetDefaultBalanceFlags(ctx, trans, org.ID, ledger.ID, alias, headers, false, true); err != nil { t.Fatalf("disable flags: %v", err) }
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, h.OutflowPayload(false, "USD", "1.00", alias))
    if err == nil && (code == 201 || code == 200) { t.Fatalf("outflow succeeded while allowSending=false: %d %s", code, string(body)) }

    // re-enable and verify outflow succeeds
    if err := h.SetDefaultBalanceFlags(ctx, trans, org.ID, ledger.ID, alias, headers, true, true); err != nil { t.Fatalf("enable flags: %v", err) }
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, h.OutflowPayload(false, "USD", "1.00", alias))
    if err != nil || (code != 201 && code != 200) { t.Fatalf("outflow after enable failed: %d %s err=%v", code, string(body), err) }
}

