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

// Flush cache via redis-cli FLUSHALL and verify caches rehydrate; no negative balances and values correct.
func TestChaos_CacheFlush_RehydrateNoNegative(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-ledger", "midaz-ledger", "midaz-valkey"}, "CacheFlush_RehydrateNoNegative")()

    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/asset/account & seed 25
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Cache Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L-cache"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "cch-" + h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure default: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"25.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"25.00"}}}}}})
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("25.00"), 10*time.Second); err != nil {
        t.Fatalf("seed wait: %v", err)
    }

    // Hit balances to warm cache
    _, _ = h.GetAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers)

    // FLUSHALL via docker exec; if unavailable, fallback to container restart to evict cache
    if _, err := h.DockerExec("midaz-valkey", "redis-cli", "FLUSHALL"); err != nil {
        // Fallback: restart Valkey to simulate eviction
        _ = h.DockerAction("restart", "midaz-valkey")
        time.Sleep(2 * time.Second)
    }

    // Read after flush: should rehydrate to 25.00 eventually and never negative
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("25.00"), 20*time.Second); err != nil {
        t.Fatalf("rehydrate wait: %v", err)
    }
}
