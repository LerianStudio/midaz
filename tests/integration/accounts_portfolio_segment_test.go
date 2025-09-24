package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Create a portfolio and segment, then create an account referencing them. Verify fields in GET.
func TestIntegration_Accounts_Portfolio_Segment_Association(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, h.LedgerPayloadRandom())
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }

    // create portfolio
    pf := map[string]any{"name": "PF " + h.RandString(4)}
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/portfolios", org.ID, ledger.ID), headers, pf)
    if err != nil || code != 201 { t.Fatalf("create portfolio: %d %s", code, string(body)) }
    var port struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &port)

    // create segment
    sg := map[string]any{"name": "SG " + h.RandString(4)}
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/segments", org.ID, ledger.ID), headers, sg)
    if err != nil || code != 201 { t.Fatalf("create segment: %d %s", code, string(body)) }
    var seg struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &seg)

    // account referencing portfolio and segment
    alias := "ps-" + h.RandString(4)
    payload := map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias, "portfolioId": port.ID, "segmentId": seg.ID}
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }
    var acc struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &acc)

    // GET account to verify portfolioId/segmentId
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", org.ID, ledger.ID, acc.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get account: %d %s", code, string(body)) }
    var raw map[string]any
    _ = json.Unmarshal(body, &raw)
    if raw["portfolioId"] != port.ID { t.Fatalf("portfolioId mismatch: want %s got %v", port.ID, raw["portfolioId"]) }
    if raw["segmentId"] != seg.ID { t.Fatalf("segmentId mismatch: want %s got %v", seg.ID, raw["segmentId"]) }
}

