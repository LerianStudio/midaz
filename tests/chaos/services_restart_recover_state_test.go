package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Restart onboarding/transaction services; APIs should return and state remains accessible.
func TestChaos_ServicesRestart_RecoverState(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-onboarding", "midaz-transaction"}, "ServicesRestart_RecoverState")()

    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create a small org/ledger to verify after restart
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Srv Org "+h.RandString(6), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // Restart onboarding and wait for health
    if err := h.RestartWithWait("midaz-onboarding", 5*time.Second); err != nil { t.Fatalf("restart onboarding: %v", err) }
    deadline := time.Now().Add(60 * time.Second)
    for {
        code, _, err = onboard.Request(ctx, "GET", "/health", headers, nil)
        if err == nil && code == 200 { break }
        if time.Now().After(deadline) { t.Fatalf("onboarding did not become healthy after restart: code=%d err=%v", code, err) }
        time.Sleep(300 * time.Millisecond)
    }
    // Verify GET still works
    code, _, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get org after onboarding restart: %d err=%v", code, err) }

    // Restart transaction and wait for health
    if err := h.RestartWithWait("midaz-transaction", 5*time.Second); err != nil { t.Fatalf("restart transaction: %v", err) }
    deadline = time.Now().Add(60 * time.Second)
    for {
        code, _, err = trans.Request(ctx, "GET", "/health", headers, nil)
        if err == nil && code == 200 { break }
        if time.Now().After(deadline) { t.Fatalf("transaction did not become healthy after restart: code=%d err=%v", code, err) }
        time.Sleep(300 * time.Millisecond)
    }
}
