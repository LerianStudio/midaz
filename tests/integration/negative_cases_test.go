package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Various negative cases for input validation and not found scenarios.
func TestIntegration_NegativeCases(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org + ledger
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	// 1) Create account missing assetCode → 400
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "type": "deposit"})
	if err != nil || code != 400 {
		t.Fatalf("expected 400 missing assetCode, got %d err=%v body=%s", code, err, string(body))
	}

	// 2) Invalid alias characters (space) → 400
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": "bad alias"})
	if err != nil || code != 400 {
		t.Fatalf("expected 400 invalid alias characters, got %d err=%v body=%s", code, err, string(body))
	}

	// 3) Prohibited external alias prefix → 400
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": "@external/USD"})
	if err != nil || code != 400 {
		t.Fatalf("expected 400 prohibited external alias prefix, got %d err=%v body=%s", code, err, string(body))
	}

	// 4) Unknown alias balances → 200 with empty items (API returns empty list)
	code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, "unknown-alias-"+h.RandString(4)), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("balances by unknown alias: code=%d err=%v body=%s", code, err, string(body))
	}

	// 5) Invalid sort_order on transactions list → 400
	code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions?sort_order=sideways", org.ID, ledger.ID), headers, nil)
	if err != nil || code != 400 {
		t.Fatalf("expected 400 invalid sort_order, got %d err=%v body=%s", code, err, string(body))
	}

	// 6) Invalid cursor format on ledgers list → 400
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers?cursor=not_a_cursor", org.ID), headers, nil)
	if err != nil || code != 400 {
		t.Fatalf("expected 400 invalid cursor, got %d err=%v body=%s", code, err, string(body))
	}
}
