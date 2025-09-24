package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Create a parent account and a child account (parentAccountId) and verify relation via GET.
func TestIntegration_Accounts_Hierarchy_ParentChild(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger + asset
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    // parent account
    parentAlias := "parent-" + h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{
        "name": "Parent", "assetCode": "USD", "type": "deposit", "alias": parentAlias,
    })
    if err != nil || code != 201 { t.Fatalf("create parent: %d %s", code, string(body)) }
    var parent struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &parent)

    // child with parentAccountId
    childAlias := "child-" + h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{
        "name": "Child", "assetCode": "USD", "type": "deposit", "alias": childAlias, "parentAccountId": parent.ID,
    })
    if err != nil || code != 201 { t.Fatalf("create child: %d %s", code, string(body)) }
    var child struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &child)

    // GET child and assert parentAccountId matches
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", org.ID, ledger.ID, child.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get child: %d %s", code, string(body)) }
    // custom unmarshal to map: easier
    var raw map[string]any
    _ = json.Unmarshal(body, &raw)
    pid, _ := raw["parentAccountId"].(string)
    if pid == "" || pid != parent.ID { t.Fatalf("expected parentAccountId=%s got %s", parent.ID, pid) }
}
