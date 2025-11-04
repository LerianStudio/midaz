package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Validates that metadata value length > 2000 on JSON endpoint is rejected with 400 and code 0051.
func TestIntegration_TransactionJSON_OversizedMetadataValue_Should400(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Org %s", h.RandString(6)))
	if err != nil || orgID == "" {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("L-%s", h.RandString(5)))
	if err != nil || ledgerID == "" {
		t.Fatalf("setup ledger: %v", err)
	}

	aliasA := fmt.Sprintf("acc-%s", h.RandString(4))
	aliasB := fmt.Sprintf("acc-%s", h.RandString(4))
	longVal := strings.Repeat("x", 2001)

	payload := map[string]any{
		"metadata": map[string]any{"note": longVal},
		"send": map[string]any{
			"asset":      "USD",
			"value":      "1",
			"source":     map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "1"}}}},
			"distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": "1"}}}},
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", orgID, ledgerID)
	code, body, reqErr := trans.Request(ctx, "POST", path, headers, payload)
	if reqErr != nil {
		t.Fatalf("request error: %v", reqErr)
	}
	if code != 400 {
		t.Fatalf("expected 400 for oversized metadata value, got %d body=%s", code, string(body))
	}

	var res map[string]any
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("unmarshal error body: %v body=%s", err, string(body))
	}
	if v, ok := res["code"].(string); !ok || v != "0051" {
		t.Fatalf("expected error code 0051 (Metadata Value Length Exceeded), got %v body=%s", res["code"], string(body))
	}
}

// Validates that metadata value length > 2000 on INFLOW endpoint is rejected with 400 and code 0051.
func TestIntegration_TransactionInflow_OversizedMetadataValue_Should400(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Org %s", h.RandString(6)))
	if err != nil || orgID == "" {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("L-%s", h.RandString(5)))
	if err != nil || ledgerID == "" {
		t.Fatalf("setup ledger: %v", err)
	}

	alias := fmt.Sprintf("acc-%s", h.RandString(4))
	longVal := strings.Repeat("y", 2001)

	payload := map[string]any{
		"metadata": map[string]any{"desc": longVal},
		"send": map[string]any{
			"asset": "USD",
			"value": "1",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "1"},
				}},
			},
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID)
	code, body, reqErr := trans.Request(ctx, "POST", path, headers, payload)
	if reqErr != nil {
		t.Fatalf("request error: %v", reqErr)
	}
	if code != 400 {
		t.Fatalf("expected 400 for oversized metadata value, got %d body=%s", code, string(body))
	}

	var res map[string]any
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("unmarshal error body: %v body=%s", err, string(body))
	}
	if v, ok := res["code"].(string); !ok || v != "0051" {
		t.Fatalf("expected error code 0051 (Metadata Value Length Exceeded), got %v body=%s", res["code"], string(body))
	}
}
