package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"
    "strings"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Multi-account integrity across chaos: batch inflows/outflows/transfers on A and B, inject restarts/pause, then reconcile balances.
func TestChaos_PostChaosIntegrity_MultiAccount(t *testing.T) {
    shouldRunChaos(t)
    // auto log capture for correlation
    defer h.StartLogCapture([]string{"midaz-ledger", "midaz-onboarding", "midaz-postgres-primary"}, "PostChaosIntegrity_MultiAccount")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/asset/accounts A and B
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Chaos Org "+h.RandString(6), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L-int"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    aliasA := "intA-" + h.RandString(4)
    aliasB := "intB-" + h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":aliasA})
    if err != nil || code != 201 { t.Fatalf("create A: %d %s", code, string(body)) }
    var accA struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &accA)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"B","assetCode":"USD","type":"deposit","alias":aliasB})
    if err != nil || code != 201 { t.Fatalf("create B: %d %s", code, string(body)) }
    var accB struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &accB)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accA.ID, headers); err != nil { t.Fatalf("ensure default A: %v", err) }
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accB.ID, headers); err != nil { t.Fatalf("ensure default B: %v", err) }
    // Enable default balances for both accounts (by alias)
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasA, headers); err != nil { t.Fatalf("enable default A: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasB, headers); err != nil { t.Fatalf("enable default B: %v", err) }

    // Seed A: 100
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"100.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset":"USD","value":"100.00"}}}}}})
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasA, "USD", headers, decimal.RequireFromString("100.00"), 10*time.Second); err != nil {
        t.Fatalf("seed wait: %v", err)
    }

    // Batch operations with resiliency: use RequestFullWithRetry to tolerate 429/502/503/504
    inA, outA, trAB, outB := 0, 0, 0, 0
    type acc struct{ Kind, ID string }
    accepted := make([]acc, 0, 64)

    // 6 inflows to A (2 each)
    for i := 0; i < 6; i++ {
        p := map[string]any{"send": map[string]any{"asset":"USD","value":"2.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset":"USD","value":"2.00"}}}}}}
        c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p, 4, 200*time.Millisecond)
        if c == 201 { inA++; var m struct{ ID string `json:"id"` }; _ = json.Unmarshal(b, &m); if m.ID != "" { accepted = append(accepted, acc{Kind: "inflowA", ID: m.ID}) } }
        if i == 2 { // inject DB pause mid-batch
            _ = h.DockerAction("pause", "midaz-postgres-primary")
            time.Sleep(1000 * time.Millisecond)
            _ = h.DockerAction("unpause", "midaz-postgres-primary")
        }
    }

    // 5 transfers A->B (1 each)
    for i := 0; i < 5; i++ {
        p := map[string]any{"send": map[string]any{
            "asset":"USD", "value":"1.00",
            "source": map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset":"USD","value":"1.00"}}}},
            "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset":"USD","value":"1.00"}}}},
        }}
        c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID), headers, p, 4, 200*time.Millisecond)
        if c == 201 { trAB++; var m struct{ ID string `json:"id"` }; _ = json.Unmarshal(b, &m); if m.ID != "" { accepted = append(accepted, acc{Kind: "transferAB", ID: m.ID}) } }
        if i == 1 { // inject service restart during transfers
            _ = h.RestartWithWait("midaz-ledger", 4*time.Second)
        }
    }

    // 3 outflows from A (1 each)
    for i := 0; i < 3; i++ {
        p := map[string]any{"send": map[string]any{"asset":"USD","value":"1.00","source": map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}}
        c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p, 4, 200*time.Millisecond)
        if c == 201 { outA++; var m struct{ ID string `json:"id"` }; _ = json.Unmarshal(b, &m); if m.ID != "" { accepted = append(accepted, acc{Kind: "outflowA", ID: m.ID}) } }
    }

    // 2 outflows from B (1 each)
    for i := 0; i < 2; i++ {
        p := map[string]any{"send": map[string]any{"asset":"USD","value":"1.00","source": map[string]any{"from": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}}
        c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p, 4, 200*time.Millisecond)
        if c == 201 { outB++; var m struct{ ID string `json:"id"` }; _ = json.Unmarshal(b, &m); if m.ID != "" { accepted = append(accepted, acc{Kind: "outflowB", ID: m.ID}) } }
    }

    // Reconcile expected finals
    expA := decimal.RequireFromString("100").Add(decimal.NewFromInt(int64(inA*2))).Sub(decimal.NewFromInt(int64(trAB))).Sub(decimal.NewFromInt(int64(outA)))
    expB := decimal.NewFromInt(int64(trAB)).Sub(decimal.NewFromInt(int64(outB)))

    gotA, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasA, "USD", headers, expA, 30*time.Second)
    if err != nil {
        // dump accepted sample
        lines := []string{}
        max := 30
        for i, a := range accepted {
            if i >= max { break }
            c, b, _ := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", org.ID, ledger.ID, a.ID), headers, nil)
            lines = append(lines, fmt.Sprintf("%d %s %s %s", c, a.Kind, a.ID, string(b)))
        }
        logPath := fmt.Sprintf("reports/logs/post_chaos_multiaccount_accepted_%d.log", time.Now().Unix())
        _ = h.WriteTextFile(logPath, strings.Join(lines, "\n"))
        t.Logf("accepted sample saved: %s (totalAccepted=%d)", logPath, len(accepted))
        t.Fatalf("A final mismatch: got=%s exp=%s err=%v (in=%d tr=%d out=%d)", gotA.String(), expA.String(), err, inA, trAB, outA)
    }
    gotB, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasB, "USD", headers, expB, 30*time.Second)
    if err != nil {
        t.Fatalf("B final mismatch: got=%s exp=%s err=%v (tr=%d out=%d)", gotB.String(), expB.String(), err, trAB, outB)
    }
}
