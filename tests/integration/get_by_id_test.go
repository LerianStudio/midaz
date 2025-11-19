package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_GetByID_OrganizationLedgerAccount(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // create organization
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    // get org by id
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s", org.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get org by id: code=%d err=%v body=%s", code, err, string(body)) }
    var orgGet struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &orgGet)
    if orgGet.ID != org.ID { t.Fatalf("org id mismatch: want %s got %s", org.ID, orgGet.ID) }

    // create ledger
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "Ledger-Get"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)

    // ensure USD asset exists before creating accounts
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
        t.Fatalf("create USD asset: %v", err)
    }

    // get ledger by id
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s", org.ID, ledger.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get ledger by id: code=%d err=%v body=%s", code, err, string(body)) }
    var ledgerGet struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledgerGet)
    if ledgerGet.ID != ledger.ID { t.Fatalf("ledger id mismatch: want %s got %s", ledger.ID, ledgerGet.ID) }

    // create account
    alias := fmt.Sprintf("getid-%s", h.RandString(4))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)

    // get account by id
    code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", org.ID, ledger.ID, account.ID), headers, nil)
    if err != nil || code != 200 { t.Fatalf("get account by id: code=%d err=%v body=%s", code, err, string(body)) }
    var accountGet struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &accountGet)
    if accountGet.ID != account.ID { t.Fatalf("account id mismatch: want %s got %s", account.ID, accountGet.ID) }
}
