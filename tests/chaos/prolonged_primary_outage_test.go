package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Prolonged primary outage: stop Postgres for ~12s while reads continue; APIs fail gracefully and recover.
func TestChaos_ProlongedPrimaryOutage_GracefulRecovery(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-transaction", "midaz-onboarding", "midaz-postgres-primary"}, "ProlongedPrimaryOutage_GracefulRecovery")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.TransactionURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create a small org to read during outage
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Outage Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // Stop primary for ~12 seconds
    if err := h.DockerAction("stop", "midaz-postgres-primary"); err != nil { t.Fatalf("stop primary: %v", err) }

    // While down, attempt reads; allow 4xx/5xx or errors, but no panics; just ensure we get responses or network errors.
    deadline := time.Now().Add(12 * time.Second)
    sawFailure := false
    for time.Now().Before(deadline) {
        code, _, err := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
        if err != nil || code >= 400 { sawFailure = true }
        time.Sleep(300 * time.Millisecond)
    }
    if !sawFailure {
        t.Logf("note: did not observe failures during outage; environment may use caches")
    }

    // Start primary and wait for health recovery
    if err := h.DockerAction("start", "midaz-postgres-primary"); err != nil { t.Fatalf("start primary: %v", err) }
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.TransactionURL+"/health", 60*time.Second)

    // Verify reads succeed again
    code, _, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get org after primary recovery: %d err=%v", code, err) }
}
