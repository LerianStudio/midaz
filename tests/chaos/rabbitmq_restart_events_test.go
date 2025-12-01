package chaos

import (
    "context"
    "encoding/json"
    "fmt"
    "testing"
    "time"

    h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Restart RabbitMQ; when events are enabled, transaction posting should continue without server 5xx.
func TestChaos_RabbitMQ_Restart_WithEvents(t *testing.T) {
    shouldRunChaos(t)

    env := h.LoadEnvironment()
    _ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
    _ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
    ctx := context.Background()
    onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
    trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
    headers := h.AuthHeaders(h.RandHex(8))

    // Setup org/ledger/asset/account
    code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("RMQ Org "+h.RandString(6), h.RandString(12)))
    if err != nil || code != 201 { t.Fatalf("create org: %d %s", code, string(body)) }
    var org struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &org)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name":"L"})
    if err != nil || code != 201 { t.Fatalf("create ledger: %d %s", code, string(body)) }
    var ledger struct{ ID string `json:"id"` }
    _ = json.Unmarshal(body, &ledger)
    if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil { t.Fatalf("asset: %v", err) }
    alias := "rmq-"+h.RandString(4)
    code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name":"A","assetCode":"USD","type":"deposit","alias":alias})
    if err != nil || code != 201 { t.Fatalf("create account: %d %s", code, string(body)) }

    // Restart RabbitMQ
    if err := h.RestartWithWait("midaz-rabbitmq", 6*time.Second); err != nil { t.Fatalf("restart rabbitmq: %v", err) }

    // Post a transaction and ensure no 5xx
    inflow := map[string]any{"send": map[string]any{"asset":"USD","value":"1.00","distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset":"USD","value":"1.00"}}}}}}
    code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, inflow)
    if err != nil || code >= 500 { t.Fatalf("inflow after rmq restart: %d %s err=%v", code, string(body), err) }
}
