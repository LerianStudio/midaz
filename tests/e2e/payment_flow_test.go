package e2e

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Payment processing flow: customer payment -> merchant settlement -> fee distribution (simple model).
// Implemented as a single inflow distributing to merchant and fee accounts, asserting balances.
func TestE2E_PaymentProcessing_SplitMerchantAndFees(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org, ledger, asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("USD asset: %v", err) }

    // Accounts: merchant and fees
    merchant := h.RandomAlias("merchant")
    fees := h.RandomAlias("fees")
    for _, al := range []string{merchant, fees} {
        accPayload := h.AccountPayloadRandom("USD", "deposit", al)
        code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, accPayload)
        if err != nil || code != 201 { t.Fatalf("create account %s: %d %s", al, code, string(body)) }
        var account struct{ ID string `json:"id"` }
        _ = json.Unmarshal(body, &account)
        if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready %s: %v", al, err) }
        if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, al, headers); err != nil { t.Fatalf("enable default %s: %v", al, err) }
    }

    // Payment 100.00 with 3% fee -> merchant 97.00, fees 3.00
    inflow := map[string]any{
        "send": map[string]any{
            "asset": "USD",
            "value": "100.00",
            "distribute": map[string]any{
                "to": []map[string]any{
                    {"accountAlias": merchant, "amount": map[string]any{"asset":"USD","value":"97.00"}},
                    {"accountAlias": fees,     "amount": map[string]any{"asset":"USD","value":"3.00"}},
                },
            },
        },
    }
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, inflow)
    if err != nil || code != 201 { t.Fatalf("inflow payment: %d %s err=%v", code, string(body), err) }

    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, merchant, "USD", headers, decimal.RequireFromString("97.00"), 5*time.Second); err != nil {
        t.Fatalf("merchant balance: %v", err)
    }
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, fees, "USD", headers, decimal.RequireFromString("3.00"), 5*time.Second); err != nil {
        t.Fatalf("fees balance: %v", err)
    }
}

