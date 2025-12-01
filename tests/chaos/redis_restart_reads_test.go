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

// Restart Redis; reads may degrade briefly but should recover; no negative balances.
func TestChaos_RedisRestart_ReadsRecover(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-ledger", "midaz-valkey"}, "RedisRestart_ReadsRecover")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup minimal org/ledger/asset/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Chaos Org "+h.RandString(6), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "cache-"+h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure default: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed and confirm
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"10.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"10.00"}}}}}})
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("10.00"), 10*time.Second); err != nil {
        t.Fatalf("seed wait: %v", err)
    }

    // Restart Redis
    if err := h.RestartWithWait("midaz-valkey", 4*time.Second); err != nil { t.Fatalf("restart redis: %v", err) }

    // Loop GET balances; allow one or two transient non-200s, but must recover quickly and never negative
    recovered := false
    deadline := time.Now().Add(10 * time.Second)
    for {
        code, b, _ := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias), headers, nil)
        if code == 200 {
            var paged struct{ Items []struct{ AssetCode string; Available decimal.Decimal } `json:"items"` }
            _ = json.Unmarshal(b, &paged)
            sum := decimal.Zero
            for _, it := range paged.Items { if it.AssetCode == "USD" { sum = sum.Add(it.Available) } }
            if sum.IsNegative() { t.Fatalf("negative balance after redis restart: %s", sum.String()) }
            recovered = true
            break
        }
        if time.Now().After(deadline) { break }
        time.Sleep(200 * time.Millisecond)
    }
    if !recovered { t.Fatalf("balances did not recover after redis restart") }
}
