package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
)

// Restart MongoDB gracefully and verify metadata operations continue to work.
func TestChaos_MongoDB_RestartGraceful(t *testing.T) {
	shouldRunChaos(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Ensure services are healthy
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
	// Create org and ledger
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Mongo Org "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-mongo"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: %d %s", code, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	// Restart Mongo
	if err := h.RestartWithWait("midaz-mongodb", 5*time.Second); err != nil {
		t.Fatalf("restart mongodb: %v", err)
	}

	// Update metadata on the ledger and read back
	upd := map[string]any{"metadata": map[string]any{"chaos": "mongo"}}
	code, body, err = onboard.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s/ledgers/%s", org.ID, ledger.ID), headers, upd)
	if err != nil || code != 200 {
		t.Fatalf("patch ledger metadata: %d %s", code, string(body))
	}

	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s", org.ID, ledger.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get ledger after mongo restart: %d %s", code, string(body))
	}
	var got struct {
		Metadata map[string]any `json:"metadata"`
	}
	_ = json.Unmarshal(body, &got)
	if got.Metadata["chaos"] != "mongo" {
		t.Fatalf("metadata not persisted after restart: %+v", got.Metadata)
	}
}
