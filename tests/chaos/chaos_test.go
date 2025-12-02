package chaos

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestChaos_RestartDatabase demonstrates a chaos experiment scaffold.
// It will restart Postgres and then verify the APIs recover gracefully.
func TestChaos_RestartDatabase(t *testing.T) {
	shouldRunChaos(t)
	// Capture logs for quick diagnostics
	defer h.StartLogCapture([]string{"midaz-ledger", "midaz-ledger", "midaz-postgres-primary"}, "RestartDatabase")()

	env := h.LoadEnvironment()
	// Ensure current health
	if err := h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second); err != nil {
		t.Fatalf("onboarding health before restart: %v", err)
	}
	if err := h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second); err != nil {
		t.Fatalf("transaction health before restart: %v", err)
	}

	// Restart primary database
	const pgPrimary = "midaz-postgres-primary"
	if err := h.RestartWithWait(pgPrimary, 6*time.Second); err != nil {
		t.Fatalf("failed to restart %s: %v", pgPrimary, err)
	}

	// Wait for services to recover
	if err := h.WaitForHTTP200(env.LedgerURL+"/health", 90*time.Second); err != nil {
		t.Fatalf("onboarding health after restart: %v", err)
	}
	if err := h.WaitForHTTP200(env.LedgerURL+"/health", 90*time.Second); err != nil {
		t.Fatalf("transaction health after restart: %v", err)
	}

	// Smoke an API call to ensure DB writes succeed
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Chaos DB "+h.RandString(6), h.RandString(12)))
	if err != nil || code >= 500 {
		t.Fatalf("create org post-restart: code=%d err=%v body=%s", code, err, string(body))
	}
	// parse to ensure valid JSON
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	if org.ID == "" {
		t.Fatalf("post-restart org ID empty; body=%s", string(body))
	}
}
