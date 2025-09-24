package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Cross-tenant transaction prevention: alias from Org A must not be usable in Org B.
func TestIntegration_MultiTenant_CrossTenantTransactionPrevention(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Org A with ledger and USD account alias
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org A: %d %s err=%v", code, string(body), err) }
    var orgA struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &orgA)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgA.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger A: %d %s err=%v", code, string(body), err) }
    var ledA struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledA)
    if err := h.CreateUSDAsset(ctx, onboard, orgA.ID, ledA.ID, headers); err != nil { t.Fatalf("asset A: %v", err) }
    aliasA := h.RandomAlias("tenantA")
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgA.ID, ledA.ID), headers, h.AccountPayloadRandom("USD", "deposit", aliasA))
    if err != nil || code != 201 { t.Fatalf("create account A: %d %s err=%v", code, string(body), err) }

    // Org B with ledger
    code, body, err = onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org B: %d %s err=%v", code, string(body), err) }
    var orgB struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &orgB)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgB.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger B: %d %s err=%v", code, string(body), err) }
    var ledB struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledB)
    if err := h.CreateUSDAsset(ctx, onboard, orgB.ID, ledB.ID, headers); err != nil { t.Fatalf("asset B: %v", err) }

    // Attempt inflow into aliasA but under orgB/ledgerB should fail (404/400)
    inflow := h.InflowPayload("USD", "1.00", aliasA)
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgB.ID, ledB.ID), headers, inflow)
    if err == nil && (code == 200 || code == 201) {
        t.Fatalf("cross-tenant inflow unexpectedly succeeded: status=%d body=%s", code, string(body))
    }
}

