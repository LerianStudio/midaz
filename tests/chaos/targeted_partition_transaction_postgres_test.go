package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Disconnect transaction from infra network; writes should fail transiently and recover upon reconnect.
func TestChaos_TargetedPartition_TransactionVsPostgres(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-transaction", "midaz-onboarding", "midaz-postgres-primary"}, "TargetedPartition_TransactionVsPostgres")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.TransactionURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/asset/account (with small retry for readiness)
    var code int; var body []byte; var err error
    for i := 0; i < 5; i++ {
        code, body, err = onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Part Org "+h.RandString(5), h.RandString(12)))
        if err == nil && code == 201 { break }
        time.Sleep(200 * time.Millisecond)
    }
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L-part"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "prt-" + h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure default: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed 10
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"10.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"10.00"}}}}}})
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("10.00"), 10*time.Second); err != nil {
        t.Fatalf("seed wait: %v", err)
    }

    // Disconnect transaction from infra-network
    if err := h.DockerNetwork("disconnect", "infra-network", "midaz-transaction"); err != nil { t.Fatalf("disconnect transaction: %v", err) }
    time.Sleep(2 * time.Second)

    // Attempt a write; expect failure (non-201 or error)
    c, _, err := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"1.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}})
    if err == nil && c == 201 {
        t.Fatalf("expected write failure during partition, got 201")
    }

    // Reconnect and retry; expect success and final=11
    if err := h.DockerNetwork("connect", "infra-network", "midaz-transaction"); err != nil { t.Fatalf("connect transaction: %v", err) }
    _ = h.WaitForHTTP200(env.TransactionURL+"/health", 30*time.Second)
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"1.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}})
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("11.00"), 20*time.Second); err != nil {
        t.Fatalf("final wait after reconnect: %v", err)
    }
}
