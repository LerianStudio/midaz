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

// E2E lifecycle: pending outflow commit then revert with numeric assertions.
func TestE2E_Lifecycle_PendingCommitThenRevert(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Org+Ledger+Account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    // Ensure USD asset exists for ledger
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }

    alias := fmt.Sprintf("e2elc-%s", h.RandString(5))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)

    // seed 12 and wait for availability
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"12.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"12.00"}}}}}})
    // wait until available reflects the seed
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("12.00"), 5*time.Second); err != nil {
        t.Fatalf("wait seed balance: %v", err)
    }

    // helper to sum available
    sumAvail := func() decimal.Decimal {
        _, b, _ := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias), headers, nil)
        var paged struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
        _ = json.Unmarshal(b, &paged)
        s := decimal.Zero
        for _, it := range paged.Items { if it.AssetCode == "USD" { s = s.Add(it.Available) } }
        return s
    }

    base := sumAvail() // 12.00

    // pending 4.25, then commit
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"pending": true, "send": map[string]any{"asset":"USD","value":"4.25","source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"4.25"}}}}}})
    if err != nil || code != 201 { t.Fatalf("pending outflow: code=%d err=%v body=%s", code, err, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)

    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/commit", org.ID, ledger.ID, tx.ID), headers, nil)
    if err != nil || code != 201 { t.Fatalf("commit: code=%d err=%v body=%s", code, err, string(body)) }
    afterCommit := sumAvail()
    wantCommit, _ := decimal.NewFromString("7.75") // 12 - 4.25
    if !afterCommit.Equal(wantCommit) { t.Fatalf("want %s got %s", wantCommit, afterCommit) }

    // revert
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/revert", org.ID, ledger.ID, tx.ID), headers, nil)
    if err != nil || (code != 201 && code != 200) { t.Fatalf("revert: code=%d err=%v body=%s", code, err, string(body)) }
    afterRevert := sumAvail()
    if !afterRevert.Equal(base) { t.Fatalf("after revert want base %s got %s", base, afterRevert) }
}

// E2E lifecycle: pending outflow cancel should not affect available balance.
func TestE2E_Lifecycle_PendingCancelNoEffect(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    // Ensure USD asset exists for ledger
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }

    alias := fmt.Sprintf("e2ec-%s", h.RandString(5))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)

    // Wait for default balance and ensure permissions are enabled
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil {
        t.Fatalf("ensure default balance ready: %v", err)
    }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
        t.Fatalf("enable default balance: %v", err)
    }
    // seed 5.50
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"5.50","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"5.50"}}}}}})

    sumAvail := func() decimal.Decimal {
        _, b, _ := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias), headers, nil)
        var paged struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
        _ = json.Unmarshal(b, &paged)
        s := decimal.Zero
        for _, it := range paged.Items { if it.AssetCode == "USD" { s = s.Add(it.Available) } }
        return s
    }
    base := sumAvail() // 5.50

    // pending 1.25, then cancel
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"pending": true, "send": map[string]any{"asset":"USD","value":"1.25","source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"1.25"}}}}}})
    if err != nil || code != 201 { t.Fatalf("pending outflow: code=%d err=%v body=%s", code, err, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)

    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/cancel", org.ID, ledger.ID, tx.ID), headers, nil)
    if err != nil || code != 201 { t.Fatalf("cancel: code=%d err=%v body=%s", code, err, string(body)) }

    after := sumAvail()
    if !after.Equal(base) { t.Fatalf("after cancel want base %s got %s", base, after) }
}
