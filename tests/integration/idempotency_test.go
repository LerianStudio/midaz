package integration

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

const (
    headerIdempotencyKey = "X-Idempotency"
    headerIdempotencyTTL = "X-TTL"
    headerReplayed       = "X-Idempotency-Replayed"
)

// Ensures second identical request with same idempotency key is replayed or rejected without double effect.
func TestIntegration_TransactionIdempotency_ReplayOrConflict(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()

    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

    // Create org, ledger, account
    headers := h.AuthHeaders(h.RandHex(8))
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }

    alias := fmt.Sprintf("cash-%s", h.RandString(5))
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"Cash","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body)) }

    idKey := "i-" + h.RandHex(6)
    inflow := map[string]any{
        "code":        fmt.Sprintf("TR-INF-%s", h.RandString(4)),
        "description": "idem inflow",
        "send": map[string]any{
            "asset": "USD",
            "value": "11.00",
            "distribute": map[string]any{
                "to": []map[string]any{{
                    "accountAlias": alias,
                    "amount":       map[string]any{"asset": "USD", "value": "11.00"},
                }},
            },
        },
    }
    reqHeaders := h.AuthHeaders(h.RandHex(8))
    reqHeaders[headerIdempotencyKey] = idKey
    reqHeaders[headerIdempotencyTTL] = "60"

    path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)

    // First call
    code, body1, err := trans.Request(ctx, "POST", path, reqHeaders, inflow)
    if err != nil || code != 201 { t.Fatalf("first inflow: code=%d err=%v body=%s", code, err, string(body1)) }

    // Give time to persist idempotency value
    time.Sleep(150 * time.Millisecond)

    // Second call with the same key and payload
    code, body2, hdr, err := trans.RequestFull(ctx, "POST", path, reqHeaders, inflow)
    if err != nil { t.Fatalf("second inflow request error: %v", err) }

    switch code {
    case 201:
        // Expect replay header true and same body
        if strings.ToLower(hdr.Get(headerReplayed)) != "true" {
            t.Fatalf("expected %s=true on replay, got %q", headerReplayed, hdr.Get(headerReplayed))
        }
        if string(body1) != string(body2) {
            t.Fatalf("replayed body mismatch")
        }
    case 409:
        // Accept conflict if value was not yet available; effect should not duplicate
        // Run a third call after a small wait to confirm replay path works
        time.Sleep(250 * time.Millisecond)
        code3, _, hdr3, err3 := trans.RequestFull(ctx, "POST", path, reqHeaders, inflow)
        if err3 != nil || code3 != 201 || strings.ToLower(hdr3.Get(headerReplayed)) != "true" {
            t.Fatalf("expected replay after conflict: code=%d hdr=%s err=%v", code3, hdr3.Get(headerReplayed), err3)
        }
    default:
        t.Fatalf("unexpected status on idempotent retry: %d body=%s", code, string(body2))
    }
}

// Using the same key but a different payload must be rejected (409).
func TestIntegration_TransactionIdempotency_ConflictOnDifferentPayload(t *testing.T) {
    env := h.LoadEnvironment()
    ctx := context.Background()

    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

    headers := h.AuthHeaders(h.RandHex(8))
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)

    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
    if err != nil || code != 201 { t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("create USD asset: %v", err) }

    alias := fmt.Sprintf("cash-%s", h.RandString(5))
    _, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"Cash","assetCode":"USD","type":"deposit","alias":alias})

    idKey := "i-" + h.RandHex(6)
    path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
    reqHeaders := h.AuthHeaders(h.RandHex(8))
    reqHeaders[headerIdempotencyKey] = idKey
    reqHeaders[headerIdempotencyTTL] = "60"

    inflowA := map[string]any{"send": map[string]any{"asset":"USD","value":"7.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"7.00"}}}}}}
    inflowB := map[string]any{"send": map[string]any{"asset":"USD","value":"8.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"8.00"}}}}}}

    // First succeeds
    code, body, err = trans.Request(ctx, "POST", path, reqHeaders, inflowA)
    if err != nil || code != 201 { t.Fatalf("first inflow: code=%d err=%v body=%s", code, err, string(body)) }

    // Second with same key but different payload → 409
    code, body, err = trans.Request(ctx, "POST", path, reqHeaders, inflowB)
    if code != 409 { t.Fatalf("expected 409 for different payload with same key, got %d body=%s", code, string(body)) }
}
