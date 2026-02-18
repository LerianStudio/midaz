package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Verifies that creating a transaction with send.value <= "0" returns HTTP 422 with code 0125.
func TestIntegration_TransactionNonPositiveValue_Returns422(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization and ledger via helpers
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Org %s", h.RandString(6)))
	if err != nil || orgID == "" {
		t.Fatalf("setup organization: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("L-%s", h.RandString(5)))
	if err != nil || ledgerID == "" {
		t.Fatalf("setup ledger: %v", err)
	}

	// Run both zero and negative value scenarios
	cases := []struct {
		name      string
		sendValue string
	}{
		{name: "zero", sendValue: "0"},
		{name: "negative", sendValue: "-1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aliasA := fmt.Sprintf("acc-%s", h.RandString(4))
			aliasB := fmt.Sprintf("acc-%s", h.RandString(4))
			payload := map[string]any{
				"send": map[string]any{
					"asset": "USD",
					"value": tc.sendValue,
					// amounts are arbitrary here because validation short-circuits on send.value
					"source":     map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "1"}}}},
					"distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": "1"}}}},
				},
			}

			path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", orgID, ledgerID)
			code, body, err := trans.Request(ctx, "POST", path, headers, payload)
			if err != nil {
				t.Fatalf("request error: %v", err)
			}
			if code != 422 {
				t.Fatalf("expected 422 for non-positive transaction value, got %d body=%s", code, string(body))
			}

			var res map[string]any
			if err := json.Unmarshal(body, &res); err != nil {
				t.Fatalf("unmarshal error body: %v body=%s", err, string(body))
			}
			if v, ok := res["code"].(string); !ok || v != "0125" {
				t.Fatalf("expected error code 0125, got %v body=%s", res["code"], string(body))
			}

			if msg, ok := res["message"].(string); !ok || msg == "" {
				t.Fatalf("expected error message present, got %v", res)
			} else if msg != "Negative or zero transaction values are not allowed. The 'send.value' must be greater than zero." {
				t.Fatalf("unexpected error message: %q", msg)
			}
		})
	}
}
