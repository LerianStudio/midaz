package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
)

func TestIntegration_OnboardingValidationErrors(t *testing.T) {
	env := h.LoadEnvironment()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))
	ctx := context.Background()

	// Create org first
	orgPayload := h.OrgPayload("X", h.RandString(8))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, orgPayload)
	if err != nil || code != 201 {
		t.Fatalf("create org for validation test failed: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// Missing required ledger.name should yield 400
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{})
	if code != 400 {
		t.Fatalf("expected 400 for missing ledger.name, got %d body=%s err=%v", code, string(body), err)
	}
}

func TestIntegration_InvalidUUIDPathParam(t *testing.T) {
	env := h.LoadEnvironment()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))
	ctx := context.Background()

	// Bad UUID should trigger 400 on GET by id
	code, body, err := onboard.Request(ctx, "GET", "/v1/organizations/not-a-uuid", headers, nil)
	if code != 400 {
		t.Fatalf("expected 400 for invalid uuid, got %d body=%s err=%v", code, string(body), err)
	}
}
