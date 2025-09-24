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

// Multi-party transactions: inflow split across three accounts, then pending outflow from all three and commit.
func TestIntegration_MultiParty_SplitInflowAndOutflow(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Org + ledger + USD asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("USD asset: %v", err) }

    // Create three accounts
    alias := [3]string{h.RandomAlias("mpa"), h.RandomAlias("mpb"), h.RandomAlias("mpc")}
    for i := 0; i < 3; i++ {
        accPayload := h.AccountPayloadRandom("USD", "deposit", alias[i])
        code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, accPayload)
        if err != nil || code != 201 { t.Fatalf("create account %d: %d %s", i, code, string(body)) }
        var account struct{ ID string `json:"id"` }
        _ = json.Unmarshal(body, &account)
        if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready %d: %v", i, err) }
        if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias[i], headers); err != nil { t.Fatalf("enable default %d: %v", i, err) }
    }

    // Inflow 100.00 split 10/20/70
    inflow := map[string]any{
        "send": map[string]any{
            "asset": "USD",
            "value": "100.00",
            "distribute": map[string]any{
                "to": []map[string]any{
                    {"accountAlias": alias[0], "amount": map[string]any{"asset":"USD","value":"10.00"}},
                    {"accountAlias": alias[1], "amount": map[string]any{"asset":"USD","value":"20.00"}},
                    {"accountAlias": alias[2], "amount": map[string]any{"asset":"USD","value":"70.00"}},
                },
            },
        },
    }
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, inflow)
    if err != nil || code != 201 { t.Fatalf("inflow split: %d %s err=%v", code, string(body), err) }

    // Wait for balances
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias[0], "USD", headers, decimal.RequireFromString("10.00"), 5*time.Second); err != nil { t.Fatalf("wait a0: %v", err) }
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias[1], "USD", headers, decimal.RequireFromString("20.00"), 5*time.Second); err != nil { t.Fatalf("wait a1: %v", err) }
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias[2], "USD", headers, decimal.RequireFromString("70.00"), 5*time.Second); err != nil { t.Fatalf("wait a2: %v", err) }

    // Pending outflow 60.00 split 5/15/40 then commit
    outflow := map[string]any{
        "pending": true,
        "send": map[string]any{
            "asset": "USD",
            "value": "60.00",
            "source": map[string]any{
                "from": []map[string]any{
                    {"accountAlias": alias[0], "amount": map[string]any{"asset":"USD","value":"5.00"}},
                    {"accountAlias": alias[1], "amount": map[string]any{"asset":"USD","value":"15.00"}},
                    {"accountAlias": alias[2], "amount": map[string]any{"asset":"USD","value":"40.00"}},
                },
            },
        },
    }
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, outflow)
    if err != nil || code != 201 { t.Fatalf("outflow pending: %d %s err=%v", code, string(body), err) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)

    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/commit", org.ID, ledger.ID, tx.ID), headers, nil)
    if err != nil || code != 201 { t.Fatalf("commit: %d %s err=%v", code, string(body), err) }

    // Expect final balances: 5/5/30 (since 10-5, 20-15, 70-40)
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias[0], "USD", headers, decimal.RequireFromString("5.00"), 5*time.Second); err != nil { t.Fatalf("wait a0 final: %v", err) }
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias[1], "USD", headers, decimal.RequireFromString("5.00"), 5*time.Second); err != nil { t.Fatalf("wait a1 final: %v", err) }
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias[2], "USD", headers, decimal.RequireFromString("30.00"), 5*time.Second); err != nil { t.Fatalf("wait a2 final: %v", err) }
}

