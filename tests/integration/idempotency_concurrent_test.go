package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "testing"
    "time"
    "os"

    "github.com/shopspring/decimal"
    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Concurrent duplicate submissions with same idempotency key should net apply once.
func TestIntegration_Idempotency_ConcurrentDuplicates(t *testing.T) {
    if os.Getenv("MIDAZ_TEST_HEAVY") != "true" && os.Getenv("MIDAZ_TEST_NIGHTLY") != "true" {
        t.Skip("heavy test; set MIDAZ_TEST_HEAVY=true to run")
    }
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s err=%v", code, string(body), err) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s err=%v", code, string(body), err) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := h.RandomAlias("idemC")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("create account: %d %s err=%v", code, string(body), err) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // Concurrent duplicate inflows with same key
    key := "kc-" + h.RandHex(6)
    amount := "9.00"
    path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
    const (
        headerIdempotencyKey = "X-Idempotency"
        headerIdempotencyTTL = "X-TTL"
    )
    payload := h.InflowPayload("USD", amount, alias)

    var wg sync.WaitGroup
    for i := 0; i < 3; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            hds := h.AuthHeaders(h.RandHex(6))
            hds[headerIdempotencyKey] = key
            hds[headerIdempotencyTTL] = "60"
            _, _, _ = trans.Request(ctx, "POST", path, hds, payload)
        }()
    }
    wg.Wait()

    // give system a moment to converge
    time.Sleep(200 * time.Millisecond)

    // Expect single net effect
    expected, _ := decimal.NewFromString(amount)
    if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, 10*time.Second); err != nil {
        t.Fatalf("concurrent idempotency net effect mismatch: %v", err)
    }
}
