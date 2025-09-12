package helpers

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
)

type balanceItem struct {
    ID        string `json:"id"`
    Key       string `json:"key"`
    AssetCode string `json:"assetCode"`
}

// EnsureDefaultBalanceRecord creates (or ensures) a default balance record with send/receive allowed.
func EnsureDefaultBalanceRecord(ctx context.Context, trans *HTTPClient, orgID, ledgerID, accountID string, headers map[string]string) error {
    payload := map[string]any{"key": "default", "allowSending": true, "allowReceiving": true}
    c, b, e := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID), headers, payload)
    if e != nil { return e }
    if c == 201 || c == 409 || (c >= 200 && c < 300) { return nil }
    return fmt.Errorf("create default balance record failed: status %d body=%s", c, string(b))
}

// EnableDefaultBalance sets AllowSending/AllowReceiving to true on the default balance for an account alias.
func EnableDefaultBalance(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias string, headers map[string]string) error {
    // Get balances by alias
    var defID string
    deadline := time.Now().Add(5 * time.Second)
    for {
        c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias), headers, nil)
        if e == nil && c == 200 {
            var paged struct{ Items []balanceItem `json:"items"` }
            _ = json.Unmarshal(b, &paged)
            for _, it := range paged.Items {
                if it.Key == "default" {
                    defID = it.ID
                    break
                }
            }
            if defID != "" { break }
        }
        if time.Now().After(deadline) { return fmt.Errorf("default balance not found for alias %s", alias) }
        time.Sleep(100 * time.Millisecond)
    }
    if defID == "" {
        return fmt.Errorf("default balance not found for alias %s", alias)
    }
    // PATCH update
    payload := map[string]any{"allowSending": true, "allowReceiving": true}
    c2, b2, e2 := trans.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, defID), headers, payload)
    if e2 != nil { return e2 }
    if c2 != 200 { return fmt.Errorf("patch default balance: status %d body=%s", c2, string(b2)) }
    return nil
}
