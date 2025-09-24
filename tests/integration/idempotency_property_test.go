package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"
    "os"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Property-like randomized test: duplicate submissions with same idempotency key must not double-apply effects.
func TestIntegration_Idempotency_RandomizedDuplicates_SingleNetEffect(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_HEAVY") != "true" && os.Getenv("MIDAZ_TEST_NIGHTLY") != "true" {
        t.Skip("heavy test; set MIDAZ_TEST_HEAVY=true to run")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("idemP")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Randomized set of keys and duplicate attempts
    type keyReq struct { key, amount string; attempts int }
    entries := []keyReq{
        {"k-"+h.RandHex(4), "1.00", 1+int(h.RandString(1)[0])%3},
        {"k-"+h.RandHex(4), "2.00", 1+int(h.RandString(1)[0])%3},
        {"k-"+h.RandHex(4), "3.00", 1+int(h.RandString(1)[0])%3},
        {"k-"+h.RandHex(4), "4.00", 1+int(h.RandString(1)[0])%3},
        {"k-"+h.RandHex(4), "5.00", 1+int(h.RandString(1)[0])%3},
    }
    // Expected net increase equals sum of unique keys' amounts (once each)
    expected := decimal.Zero
    for _, e := range entries { v, _ := decimal.NewFromString(e.amount); expected = expected.Add(v) }

    path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
    const (
        headerIdempotencyKey = "X-Idempotency"
        headerIdempotencyTTL = "X-TTL"
        headerReplayed       = "X-Idempotency-Replayed"
    )

    // Execute requests with duplicates for each key
    for _, e := range entries {
        payload := h.InflowPayload("USD", e.amount, alias)
        hds := h.AuthHeaders(h.RandHex(6))
        hds[headerIdempotencyKey] = e.key
        hds[headerIdempotencyTTL] = "60"

        // First attempt
        c, b, err := trans.Request(ctx, "POST", path, hds, payload)
        if err != nil || c != 201 { t.Fatalf("first inflow %s: %d %s err=%v", e.key, c, string(b), err) }
        // duplicates
        for i := 1; i < e.attempts; i++ {
            time.Sleep(50 * time.Millisecond)
            c2, b2, hdr2, err2 := trans.RequestFull(ctx, "POST", path, hds, payload)
            if err2 != nil { t.Fatalf("dup inflow err: %v", err2) }
            switch c2 {
            case 201:
                // must indicate replay
                if hdr2.Get(headerReplayed) == "" { t.Fatalf("missing replay header on duplicate apply: key=%s", e.key) }
            case 409:
                // eventual replay on a subsequent call should succeed
                time.Sleep(100 * time.Millisecond)
                c3, _, hdr3, err3 := trans.RequestFull(ctx, "POST", path, hds, payload)
                if err3 != nil || c3 != 201 || hdr3.Get(headerReplayed) == "" {
                    t.Fatalf("expected replay after 409: code=%d hdr=%s err=%v body=%s", c3, hdr3.Get(headerReplayed), err3, string(b2))
                }
            default:
                t.Fatalf("unexpected status duplicate: %d body=%s", c2, string(b2))
            }
        }
    }

    // Verify final available equals expected
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, 10*time.Second); err != nil {
        t.Fatalf("idempotency net effect mismatch: %v", err)
    }
}
