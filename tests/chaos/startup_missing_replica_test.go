package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Start onboarding with replica unavailable; service should not become healthy and reads should fail.
func TestChaos_Startup_MissingReplica_NoPanic(t *testing.T) {
	shouldRunChaos(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Ensure an org exists for later GET
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("StartRep Org "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// Stop replica and restart onboarding
	_ = h.DockerAction("stop", "midaz-postgres-replica")
	_ = h.DockerAction("restart", "midaz-onboarding")

	// Onboarding should NOT become healthy without replica
	if err := h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second); err == nil {
		t.Fatalf("onboarding unexpectedly healthy without replica")
	}

	// Read should NOT work (reads are routed to replica)
	code, _, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
	if err == nil && code == 200 {
		t.Fatalf("expected GET org to fail or not return 200 without replica, got 200")
	}

	// Start replica back
	_ = h.DockerAction("start", "midaz-postgres-replica")
}
