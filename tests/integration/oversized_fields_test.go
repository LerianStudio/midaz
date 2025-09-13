package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Covers non-metadata oversized fields (name/alias, etc.) returning 400.
func TestIntegration_OversizedFields_Should400(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Create minimal org for nesting resources
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
    if err != nil || code != 201 {
        t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
    }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // 1) Ledger name > 256 → 400
    longName := strings.Repeat("A", 257)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": longName})
    if code != 400 {
        t.Fatalf("expected 400 for oversized ledger.name, got %d body=%s err=%v", code, string(body), err)
    }

    // Create a valid ledger to test account alias length
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L" + h.RandString(4)})
    if err != nil || code != 201 {
        t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
    }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    // Ensure USD asset exists to avoid downstream constraints
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
        t.Fatalf("create USD asset: %v", err)
    }

    // 2) Account alias > 100 → 400
    longAlias := strings.Repeat("a", 101)
    payload := map[string]any{
        "name":      "A",
        "assetCode": "USD",
        "type":      "deposit",
        "alias":     longAlias,
    }
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
    if err != nil || code != 400 {
        t.Fatalf("expected 400 for oversized account.alias, got %d err=%v body=%s", code, err, string(body))
    }
}

