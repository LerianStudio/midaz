package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "sync"
    "testing"
    "time"
    "strings"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Guard so chaos tests only run when explicitly requested.
func shouldRunChaos(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_CHAOS") != "true" {
        t.Skip("set MIDAZ_TEST_CHAOS=true to run chaos tests")
    }
}

// Restart PostgreSQL primary during a stream of writes; system should recover and final balance should match the net effect of successful operations.
func TestChaos_PostgresRestart_DuringWrites(t *testing.T) {
    shouldRunChaos(t)
    if os.Getenv("MIDAZ_TEST_CHAOS_STRICT") != "true" {
        t.Skip("skipping postgres restart chaos (strict) unless MIDAZ_TEST_CHAOS_STRICT=true")
    }
    // auto log capture for correlation
    defer h.StartLogCapture([]string{"midaz-transaction", "midaz-onboarding", "midaz-postgres-primary"}, "PostgresRestart_DuringWrites")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.TransactionURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/asset/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Chaos Org "+h.RandString(6), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "chaos-"+h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure default: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }
    // Seed
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"100.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"100.00"}}}}}})
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("100.00"), 10*time.Second); err != nil {
        t.Fatalf("seed wait: %v", err)
    }

    // Start parallel writers
    var wg sync.WaitGroup
    inSucc, outSucc := 0, 0
    mu := sync.Mutex{}
    type acceptedRec struct{ Kind, ID string }
    accepted := make([]acceptedRec, 0, 256)
    stop := make(chan struct{})
    inflow := func(val string) {
        defer wg.Done()
        for {
            select { case <-stop: return; default: }
            p := map[string]any{"send": map[string]any{"asset":"USD","value":val,"distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": val}}}}}}
            c, b, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, p)
            if c == 201 {
                var m struct{ ID string `json:"id"` }
                _ = json.Unmarshal(b, &m)
                mu.Lock()
                inSucc++
                if m.ID != "" { accepted = append(accepted, acceptedRec{Kind: "inflow", ID: m.ID}) }
                mu.Unlock()
            }
            time.Sleep(20 * time.Millisecond)
        }
    }
    outflow := func(val string) {
        defer wg.Done()
        for {
            select { case <-stop: return; default: }
            p := map[string]any{"send": map[string]any{"asset":"USD","value":val,"source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": val}}}}}}
            c, b, _ := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, p)
            if c == 201 {
                var m struct{ ID string `json:"id"` }
                _ = json.Unmarshal(b, &m)
                mu.Lock()
                outSucc++
                if m.ID != "" { accepted = append(accepted, acceptedRec{Kind: "outflow", ID: m.ID}) }
                mu.Unlock()
            }
            time.Sleep(30 * time.Millisecond)
        }
    }

    wg.Add(2)
    go inflow("2.00")
    go outflow("1.00")

    // Restart Postgres primary mid-flight
    if err := h.RestartWithWait("midaz-postgres-primary", 5*time.Second); err != nil {
        close(stop)
        wg.Wait()
        t.Fatalf("restart postgres primary: %v", err)
    }

    // Let writers continue briefly, then stop
    time.Sleep(3 * time.Second)
    close(stop)
    wg.Wait()

    // Compute expected and verify eventual convergence
    expected := decimal.RequireFromString("100").Add(decimal.NewFromInt(int64(inSucc*2))).Sub(decimal.NewFromInt(int64(outSucc*1)))
    got, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, 20*time.Second)
    if err != nil {
        // Correlate accepted IDs by fetching their final statuses
        lines := []string{}
        max := 20
        for i, a := range accepted {
            if i >= max { break }
            c, b, _ := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", org.ID, ledger.ID, a.ID), headers, nil)
            lines = append(lines, fmt.Sprintf("%d %s %s %s", c, a.Kind, a.ID, string(b)))
        }
        logPath := fmt.Sprintf("reports/logs/postgres_restart_writes_accepted_%d.log", time.Now().Unix())
        _ = h.WriteTextFile(logPath, strings.Join(lines, "\n"))
        t.Logf("accepted sample saved: %s (totalAccepted=%d)", logPath, len(accepted))
        t.Fatalf("final mismatch after restart: got=%s expected=%s err=%v (inSucc=%d outSucc=%d)", got.String(), expected.String(), err, inSucc, outSucc)
    }
}
