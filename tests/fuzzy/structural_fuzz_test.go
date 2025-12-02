package fuzzy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestFuzz_Structural_OmittedUnknownInvalidJSONLarge(t *testing.T) {
	shouldRun(t)
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create org for context
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Struct Org "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// Omitted required fields (ledger name)
	c, b, _ := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{})
	if c < 400 || c >= 500 {
		t.Fatalf("expected 4xx for omitted required fields; got %d body=%s", c, string(b))
	}

	// Unknown fields in ledger payload
	c, b, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L", "unknown": "x"})
	if c < 400 || c >= 500 {
		t.Fatalf("expected 4xx for unknown fields; got %d body=%s", c, string(b))
	}

	// Invalid JSON
	raw := []byte("{ invalid json }")
	c, b, _, _ = onboard.RequestRaw(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), map[string]string{"X-Request-Id": h.RandHex(6), "Authorization": headers["Authorization"]}, "application/json", raw)
	if c < 400 || c >= 500 {
		t.Fatalf("expected 4xx for invalid json; got %d body=%s", c, string(b))
	}

	// Large body (oversized metadata value) ~250KB
	large := strings.Repeat("A", 250*1024)
	payload := map[string]any{"name": "L2", "metadata": map[string]any{"blob": large}}
	c, b, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, payload)
	if c >= 500 {
		t.Fatalf("server 5xx on large body: body=%s", string(b))
	}
}
