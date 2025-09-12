package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// commit on non-pending (e.g., approved/created) should return 400
func TestIntegration_Lifecycle_CommitNonPending_Should400(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger + account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }
    alias := fmt.Sprintf("cmt-%s", h.RandString(5))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account.ID, headers); err != nil { t.Fatalf("ensure default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // create normal inflow (non-pending) which should become APPROVED
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"1.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}})
    if err != nil || code != 201 { t.Fatalf("inflow: code=%d err=%v body=%s", code, err, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)

    // attempting to commit approved/non-pending should be a client error (accept 400 or 422)
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/commit", org.ID, ledger.ID, tx.ID), headers, nil)
    if !(code == 400 || code == 422) { t.Fatalf("expected 400/422 committing non-pending, got %d body=%s", code, string(body)) }
}

// revert on non-approved should return 400
func TestIntegration_Lifecycle_RevertNonApproved_Should400(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // org + ledger + account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }
    alias := fmt.Sprintf("rv-%s", h.RandString(5))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }
    var account2 struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &account2)
    if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, account2.ID, headers); err != nil { t.Fatalf("ensure default ready: %v", err) }
    if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil { t.Fatalf("enable default: %v", err) }

    // seed some funds to allow pending outflow
    _, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset":"USD","value":"2.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"2.00"}}}}}})

    // create PENDING outflow
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), headers, map[string]any{"pending": true, "send": map[string]any{"asset":"USD","value":"1.00","source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}})
    if err != nil || code != 201 { t.Fatalf("pending outflow: code=%d err=%v body=%s", code, err, string(body)) }
    var tx struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &tx)

    // revert should be a client error (non-approved) → accept 400 or 422
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/revert", org.ID, ledger.ID, tx.ID), headers, nil)
    if !(code == 400 || code == 422) { t.Fatalf("expected 400/422 reverting non-approved, got %d body=%s", code, string(body)) }
}
