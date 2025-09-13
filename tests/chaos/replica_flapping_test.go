package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Stop/start the Postgres replica repeatedly; reads should continue via primary without crashes.
func TestChaos_ReplicaFlapping_ReadsContinue(t *testing.T) {
    shouldRunChaos(t)
    defer h.StartLogCapture([]string{"midaz-onboarding", "midaz-postgres-replica", "midaz-postgres-primary"}, "ReplicaFlapping_ReadsContinue")()

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.TransactionURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create org and two ledgers to read during flapping
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Replica Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    for i := 0; i < 2; i++ {
        _, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%d", i)})
    }

    // Flap the replica 3 times; assert recovery after restart
    for i := 0; i < 3; i++ {
        if err := h.DockerAction("stop", "midaz-postgres-replica"); err != nil { t.Fatalf("stop replica: %v", err) }
        time.Sleep(2 * time.Second)
        if err := h.DockerAction("start", "midaz-postgres-replica"); err != nil { t.Fatalf("start replica: %v", err) }
        // After start, ensure reads recover
        deadline := time.Now().Add(15 * time.Second)
        for {
            code, _, err := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, nil)
            if err == nil && code == 200 { break }
            if time.Now().After(deadline) { t.Fatalf("reads did not recover after replica start") }
            time.Sleep(200 * time.Millisecond)
        }
    }
}
