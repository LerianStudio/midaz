package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// This test only runs when RABBITMQ_TRANSACTION_ASYNC=true in the service configuration and
// TEST_VERIFY_EVENTS=true is provided to the test process. It performs a sanity request to ensure
// transaction creation still behaves with async enabled. Deep queue verification is out of scope here.
func TestIntegration_EventsAsync_Sanity(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()

	// Create test isolation helper
	isolation := h.NewTestIsolation()

	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := isolation.MakeTestHeaders()

	orgName := isolation.UniqueOrgName("EventsAsync")
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(orgName, h.RandString(14)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil {
		t.Fatalf("failed to unmarshal org response: %v body: %s", err, string(body))
	}
	ledgerName := isolation.UniqueLedgerName("L")
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": ledgerName})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil {
		t.Fatalf("failed to unmarshal ledger response: %v body: %s", err, string(body))
	}
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := isolation.UniqueAccountAlias("ev")
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}

	// Sanity inflow; if async enabled, service must still respond 201.
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}})
	if err != nil || code != 201 {
		t.Fatalf("inflow under async: code=%d err=%v body=%s", code, err, string(body))
	}
}
