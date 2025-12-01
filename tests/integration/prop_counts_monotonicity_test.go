package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "strconv"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Monotonicity of count metrics (HEAD X-Total-Count) for ledgers and accounts within a single org.
func TestProperty_Monotonic_CountMetrics(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create isolated org
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Prop Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // Helper to get HEAD count and ensure non-decreasing behavior
    getCount := func(path string) (int, error) {
        c, _, hdr, e := onboard.RequestFull(ctx, "HEAD", path, headers, nil)
        if e != nil || c != 204 { return -1, fmt.Errorf("head %s: code=%d err=%v", path, c, e) }
        v, err := strconv.Atoi(hdr.Get("X-Total-Count"))
        if err != nil { return -1, err }
        return v, nil
    }

    // Ledgers: create 3 and assert counts never decrease between observations
    last, err := getCount(fmt.Sprintf("/v1/organizations/%s/ledgers/metrics/count", org.ID))
    if err != nil { t.Fatalf("ledger head initial: %v", err) }

    for i := 0; i < 3; i++ {
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%d-%s", i, h.RandString(3))})
        if e != nil || c != 201 { t.Fatalf("create ledger %d: %d %s", i, c, string(b)) }

        // poll head until it reaches at least last+1 or times out; ensure never decreases
        deadline := time.Now().Add(3 * time.Second)
        for {
            cur, err := getCount(fmt.Sprintf("/v1/organizations/%s/ledgers/metrics/count", org.ID))
            if err == nil {
                if cur < last { t.Fatalf("ledger count decreased: last=%d cur=%d", last, cur) }
                if cur >= last+1 { last = cur; break }
            }
            if time.Now().After(deadline) { t.Fatalf("ledger count did not increase in time; last=%d", last) }
            time.Sleep(100 * time.Millisecond)
        }
    }

    // Accounts: create 4 under a new ledger; head should be non-decreasing and increase as we add accounts
    c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-Accounts"})
    if e != nil || c != 201 { t.Fatalf("create ledger for accounts: %d %s", c, string(b)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(b, &ledger)
    // Prepare USD asset (accounts require it)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }

    lastAcc, err := getCount(fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledger.ID))
    if err != nil { t.Fatalf("accounts head initial: %v", err) }

    for i := 0; i < 4; i++ {
        alias := fmt.Sprintf("pa-%d-%s", i, h.RandString(3))
        c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
        if e != nil || c != 201 { t.Fatalf("create account %d: %d %s", i, c, string(b)) }

        deadline := time.Now().Add(3 * time.Second)
        for {
            cur, err := getCount(fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledger.ID))
            if err == nil {
                if cur < lastAcc { t.Fatalf("accounts count decreased: last=%d cur=%d", lastAcc, cur) }
                if cur >= lastAcc+1 { lastAcc = cur; break }
            }
            if time.Now().After(deadline) { t.Fatalf("accounts count did not increase in time; last=%d", lastAcc) }
            time.Sleep(100 * time.Millisecond)
        }
    }
}

