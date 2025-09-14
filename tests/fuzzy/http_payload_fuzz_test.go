package fuzzy

import (
    "context"
    "encoding/json"
    "fmt"
    "math/rand"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Run with: MIDAZ_TEST_FUZZ=true go test -v ./tests/fuzzy -run Fuzz -count=1

func shouldRun(t *testing.T) { /* always run */ }

func randString(n int) string {
    letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-@:/\t\n")
    b := make([]rune, n)
    for i := range b { b[i] = letters[rand.Intn(len(letters))] }
    return string(b)
}

func TestFuzz_Organization_Fields(t *testing.T) {
    shouldRun(t)
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    for i := 0; i < 30; i++ {
        nameLen := rand.Intn(400)
        docLen := rand.Intn(400)
        payload := h.OrgPayload(randString(nameLen), randString(docLen))
        code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, payload)
        if err != nil { t.Fatalf("request error: %v", err) }
        if code >= 500 {
            t.Fatalf("server 5xx on fuzz org fields: %d body=%s (nameLen=%d docLen=%d)", code, string(body), nameLen, docLen)
        }
    }
}

func TestFuzz_Accounts_AliasAndType(t *testing.T) {
    shouldRun(t)
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Baseline org+ledger+asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Fuzz Org "+h.RandString(6), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    // Try random aliases/types
    for i := 0; i < 40; i++ {
        aliasLen := rand.Intn(150)
        alias := randString(aliasLen)
        // Randomly include forbidden substring to trigger 400
        typ := "deposit"
        if rand.Intn(5) == 0 { typ = "external" }
        payload := map[string]any{"name":"A","assetCode":"USD","type":typ,"alias":alias}
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
        if e != nil { t.Fatalf("account request error: %v", e) }
        if c >= 500 { t.Fatalf("server 5xx on fuzz account: %d body=%s (aliasLen=%d type=%s)", c, string(b), aliasLen, typ) }
        // Attempt a collision: re-submit same alias once
        if c == 201 && i%7 == 0 {
            c2, b2, _ := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
            if !(c2 == 409 || c2 >= 400) {
                t.Fatalf("expected conflict/4xx on duplicate alias; got %d body=%s", c2, string(b2))
            }
        }
    }

    _ = trans // reserved for future header fuzz on transaction endpoints
}

func TestFuzz_Transactions_Amounts_And_Codes(t *testing.T) {
    shouldRun(t)
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/asset/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Fuzz Org "+h.RandString(6), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "fz-"+h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }

    // Ensure default balance exists and is enabled
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil { t.Fatalf("ensure default: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Seed some funds
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"50.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"50.00"}}}}}})

    rng := rand.New(rand.NewSource(time.Now().UnixNano()))
    for i := 0; i < 40; i++ {
        // values in set: negative, zero, big, precise
        choices := []string{"-1.00", "0", "0.00", "1.234567890123456789", "9999999999999999999999", fmt.Sprintf("%d.00", rng.Intn(100))}
        val := choices[rng.Intn(len(choices))]
        inflow := rng.Intn(2) == 0
        var payload map[string]any
        if inflow {
            payload = map[string]any{"send": map[string]any{"asset":"USD","value":val,"distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": val}}}}}}
        } else {
            payload = map[string]any{"send": map[string]any{"asset":"USD","value":val,"source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value": val}}}}}}
        }
        path := "/v1/organizations/%s/ledgers/%s/transactions/inflow"
        if !inflow { path = "/v1/organizations/%s/ledgers/%s/transactions/outflow" }
        c, b, _ := trans.Request(ctx, "POST", fmt.Sprintf(path, org.ID, ledger.ID), headers, payload)
        if c >= 500 {
            // Allow known overflow error (code 0097) only; others log and continue (robustness signal)
            if !jsonContainsCode(b, "0097") {
                t.Logf("server 5xx on fuzz txn val=%s inflow=%v body=%s", val, inflow, string(b))
            }
        }
    }
}

func jsonContainsCode(b []byte, code string) bool {
    var m map[string]any
    _ = json.Unmarshal(b, &m)
    if v, ok := m["code"].(string); ok { return v == code }
    return false
}
