package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "strconv"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_AccountAliasUniqueness_Conflict(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    alias := fmt.Sprintf("dup-%s", h.RandString(5))
    payload := map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias}
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }

    // duplicate alias in same ledger should conflict
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
    if code != 409 { t.Fatalf("expected 409 for duplicated alias, got %d body=%s", code, string(body)) }
}

func TestIntegration_AccountsHeadCount(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    // head count before
    code, _, hdr, err := onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 204 { t.Fatalf("accounts head before: code=%d err=%v", code, err) }
    before, _ := strconv.Atoi(hdr.Get("X-Total-Count"))

    // create two accounts
    for i := 0; i < 2; i++ {
        alias := fmt.Sprintf("u-%d-%s", i, h.RandString(3))
        _, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    }

    code, _, hdr, err = onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 204 { t.Fatalf("accounts head after: code=%d err=%v", code, err) }
    after, convErr := strconv.Atoi(hdr.Get("X-Total-Count"))
    if convErr != nil || after < before+2 { t.Fatalf("accounts head expected increase by >=2, before=%d after=%d", before, after) }
}
