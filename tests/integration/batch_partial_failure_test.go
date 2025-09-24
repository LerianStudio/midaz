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

// Batch-like sequence with one intentional failure: verify only successful operations affect balance.
func TestIntegration_Batch_PartialFailure_NoSideEffects(t *testing.T) {
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

    aliasOK := h.RandomAlias("batchOK")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", aliasOK))
    if err != nil || code != 201 { t.Fatalf("create account ok: %d %s err=%v", code, string(body), err) }
    var accountOK struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &accountOK)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accountOK.ID, headers); err != nil { t.Fatalf("default ok: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasOK, headers); err != nil { t.Fatalf("enable ok: %v", err) }

    // seed 20.00
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, h.InflowPayload("USD", "20.00", aliasOK))
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasOK, "USD", headers, decimal.RequireFromString("20.00"), 5*time.Second); err != nil { t.Fatalf("seed wait: %v", err) }

    // sequence: outflows 1,1,1, fail(alias missing), 1,1 => expected deduction = 1+1+1+1+1 = 5.00
    steps := []struct{ alias string; amount string }{
        {aliasOK, "1.00"}, {aliasOK, "1.00"}, {aliasOK, "1.00"}, {"missing-" + h.RandString(4), "1.00"}, {aliasOK, "1.00"}, {aliasOK, "1.00"},
    }
    success := decimal.Zero
    for i, s := range steps {
        payload := h.OutflowPayload(false, "USD", s.amount, s.alias)
        c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, payload)
        if s.alias == aliasOK {
            if e != nil || c != 201 { t.Fatalf("step %d expected success: %d %s", i, c, string(b)) }
            v, _ := decimal.NewFromString(s.amount)
            success = success.Add(v)
        } else {
            if c < 400 { t.Fatalf("step %d expected failure for bad alias, got %d", i, c) }
        }
    }

    // verify final available = 20.00 - success
    expected := decimal.RequireFromString("20.00").Sub(success)
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasOK, "USD", headers, expected, 5*time.Second); err != nil {
        t.Fatalf("batch partial failure balance mismatch: %v", err)
    }
}

