package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Same account alias should be allowed in different tenants (organizations).
func TestIntegration_MultiTenant_SameAliasAllowedAcrossTenants(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    alias := "same-" + h.RandString(6)

    // Org A
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org A: %d %s", code, string(body)) }
    var orgA struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &orgA)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgA.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("ledger A: %d %s", code, string(body)) }
    var ledA struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledA)
    if err := h.CreateUSDAsset(ctx, onboard, orgA.ID, ledA.ID, headers); err != nil { t.Fatalf("asset A: %v", err) }
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgA.ID, ledA.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("account A: %d %s", code, string(body)) }

    // Org B
    code, body, err = onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org B: %d %s", code, string(body)) }
    var orgB struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &orgB)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgB.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("ledger B: %d %s", code, string(body)) }
    var ledB struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledB)
    if err := h.CreateUSDAsset(ctx, onboard, orgB.ID, ledB.ID, headers); err != nil { t.Fatalf("asset B: %v", err) }
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgB.ID, ledB.ID), headers, h.AccountPayloadRandom("USD", "deposit", alias))
    if err != nil || code != 201 { t.Fatalf("account B with same alias failed unexpectedly: %d %s", code, string(body)) }
}

