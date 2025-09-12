package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "testing"
    "time"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Parallel inflow/outflow contention on same account without negative balances.
func TestIntegration_ParallelContention_NoNegativeBalance(t *testing.T) {
    t.Parallel()
    env := h.LoadEnvironment()
    ctx := context.Background()

    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup: org, ledger, asset, account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-"+h.RandString(4)})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create asset: %v", err) }

    alias := fmt.Sprintf("acct-%s", h.RandString(6))
    accPayload := map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias}
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, accPayload)
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)

    // Wait for default balance and ensure permissions are enabled
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure default balance ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default balance: %v", err) }

    // Seed initial balance via inflow: 500.00
    inflow := func(val string) (int, []byte, error) {
        p := map[string]any{
            "code": "INF-"+h.RandString(6),
            "send": map[string]any{
                "asset": "USD", "value": val,
                "distribute": map[string]any{
                    "to": []map[string]any{{
                        "accountAlias": alias,
                        "amount": map[string]any{"asset": "USD", "value": val},
                    }},
                },
            },
        }
        return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
    }
    code, body, err = inflow("500.00")
    if err != nil || code != 201 { t.Fatalf("seed inflow: code=%d err=%v body=%s", code, err, string(body)) }

    // Wait for available == 500.00
    target, _ := decimal.NewFromString("500.00")
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, target, 10*time.Second); err != nil {
        t.Fatalf("wait seed balance: %v", err)
    }

    // Prepare parallel operations: 40 outflows of 5.00 (total 200), 20 inflows of 3.00 (total 60)
    var wg sync.WaitGroup
    outSucc := int64(0)
    inSucc := int64(0)
    mu := sync.Mutex{}

    outflow := func(val string) (int, []byte, error) {
        p := map[string]any{
            "code": "OUT-"+h.RandString(6),
            "send": map[string]any{
                "asset": "USD", "value": val,
                "source": map[string]any{
                    "from": []map[string]any{{
                        "accountAlias": alias,
                        "amount": map[string]any{"asset": "USD", "value": val},
                    }},
                },
            },
        }
        return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
    }

    // launch outflows
    for i := 0; i < 40; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            c, _, _ := outflow("5.00")
            if c == 201 { mu.Lock(); outSucc++; mu.Unlock() }
        }()
    }
    // launch inflows
    for i := 0; i < 20; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            c, _, _ := inflow("3.00")
            if c == 201 { mu.Lock(); inSucc++; mu.Unlock() }
        }()
    }
    wg.Wait()

    // Expected final = 500 - 40*5 + 20*3 = 500 - 200 + 60 = 360
    expected := decimal.NewFromInt(500).Sub(decimal.NewFromInt(40*5)).Add(decimal.NewFromInt(20*3))
    got, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, 15*time.Second)
    if err != nil {
        t.Fatalf("final balance mismatch: got=%s expected=%s err=%v (inSucc=%d outSucc=%d)", got.String(), expected.String(), err, inSucc, outSucc)
    }

    if got.IsNegative() {
        t.Fatalf("balance went negative: %s", got.String())
    }
}

// Burst of mixed operations with deterministic final balances, and overshoot check avoids negatives.
func TestIntegration_BurstMixedOperations_DeterministicFinal(t *testing.T) {
    t.Parallel()
    env := h.LoadEnvironment()
    ctx := context.Background()

    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/assets/accounts A and B
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-"+h.RandString(5)})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create asset: %v", err) }

    aAlias := fmt.Sprintf("acc-a-%s", h.RandString(5))
    bAlias := fmt.Sprintf("acc-b-%s", h.RandString(5))
    for _, alias := range []string{aAlias, bAlias} {
        p := map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias}
        code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, p)
        if err != nil || code != 201 { t.Fatalf("create account %s: code=%d err=%v body=%s", alias, code, err, string(body)) }
        var acc struct{ ID string `json:"id"` }
        _ = json.Unmarshal(body, &acc)
        if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure balance %s ready: %v", alias, err) }
        if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable balance %s: %v", alias, err) }
    }

    // Seed A with 500
    seed := func(alias, amt string) {
        p := map[string]any{"send": map[string]any{"asset": "USD", "value": amt, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": amt}}}}}}
        c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
        if e != nil || c != 201 { t.Fatalf("seed inflow %s: code=%d err=%v body=%s", alias, c, e, string(b)) }
    }
    seed(aAlias, "500.00")
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aAlias, "USD", headers, decimal.RequireFromString("500.00"), 10*time.Second); err != nil {
        t.Fatalf("wait seed A: %v", err)
    }
    // B should be zero
    if cur, err := h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, bAlias, "USD", headers); err != nil || !cur.Equal(decimal.Zero) {
        t.Fatalf("initial B not zero: cur=%s err=%v", cur.String(), err)
    }

    // Define operations
    jsonTransfer := func(fromAlias, toAlias, val string) (int, []byte, error) {
        p := map[string]any{
            "send": map[string]any{
                "asset": "USD", "value": val,
                "source": map[string]any{"from": []map[string]any{{"accountAlias": fromAlias, "amount": map[string]any{"asset": "USD", "value": val}}}},
                "distribute": map[string]any{"to": []map[string]any{{"accountAlias": toAlias, "amount": map[string]any{"asset": "USD", "value": val}}}},
            },
        }
        return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID), headers, p)
    }
    outflow := func(alias, val string) (int, []byte, error) {
        p := map[string]any{"send": map[string]any{"asset": "USD", "value": val, "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}}
        return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
    }
    inflow := func(alias, val string) (int, []byte, error) {
        p := map[string]any{"send": map[string]any{"asset": "USD", "value": val, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}}}}
        return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
    }

    // Launch burst: 60 transfers A->B of 1.00 (<= seed), 20 outflows of 5.00, 30 inflows of 1.00
    var wg sync.WaitGroup
    trSucc, outSucc, inSucc := 0, 0, 0
    mu := sync.Mutex{}
    for i := 0; i < 60; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            c, _, _ := jsonTransfer(aAlias, bAlias, "1.00")
            if c == 201 { mu.Lock(); trSucc++; mu.Unlock() }
        }()
    }
    for i := 0; i < 20; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            c, _, _ := outflow(aAlias, "5.00")
            if c == 201 { mu.Lock(); outSucc++; mu.Unlock() }
        }()
    }
    for i := 0; i < 30; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            c, _, _ := inflow(aAlias, "1.00")
            if c == 201 { mu.Lock(); inSucc++; mu.Unlock() }
        }()
    }
    wg.Wait()

    // Expected final A = 500 - trSucc*1 - outSucc*5 + inSucc*1
    expA := decimal.RequireFromString("500").
        Sub(decimal.NewFromInt(int64(trSucc))).
        Sub(decimal.NewFromInt(int64(outSucc*5))).
        Add(decimal.NewFromInt(int64(inSucc)))

    // Wait for A
    gotA, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aAlias, "USD", headers, expA, 20*time.Second)
    if err != nil {
        t.Fatalf("A final mismatch: got=%s exp=%s err=%v (tr=%d out=%d in=%d)", gotA.String(), expA.String(), err, trSucc, outSucc, inSucc)
    }
    if gotA.IsNegative() { t.Fatalf("A negative final balance: %s", gotA.String()) }

    // Expected final B = trSucc*1
    expB := decimal.NewFromInt(int64(trSucc))
    gotB, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, bAlias, "USD", headers, expB, 20*time.Second)
    if err != nil {
        t.Fatalf("B final mismatch: got=%s exp=%s err=%v (tr=%d)", gotB.String(), expB.String(), err, trSucc)
    }
    if gotB.IsNegative() { t.Fatalf("B negative final balance: %s", gotB.String()) }
}
