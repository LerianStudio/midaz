package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Start onboarding with replica DNS unavailable; ensure no panic and eventual health.
func TestChaos_Startup_MissingReplica_NoPanic(t *testing.T) {
    shouldRunChaos(t)

    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Ensure an org exists for later GET
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("StartRep Org "+h.RandString(5), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // Stop replica and restart onboarding
    _ = h.DockerAction("stop", "midaz-postgres-replica")
    _ = h.DockerAction("restart", "midaz-onboarding")

    // Onboarding should eventually become healthy
    if err := h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second); err != nil {
        t.Fatalf("onboarding not healthy after restart without replica: %v", err)
    }

    // Read should work
    code, _, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get org after onboarding restart (no replica): %d err=%v", code, err) }

    // Start replica back
    _ = h.DockerAction("start", "midaz-postgres-replica")
}
