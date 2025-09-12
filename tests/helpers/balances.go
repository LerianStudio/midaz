package helpers

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/shopspring/decimal"
)

type balanceItem struct {
    ID        string `json:"id"`
    Key       string `json:"key"`
    AssetCode string `json:"assetCode"`
}

// EnsureDefaultBalanceRecord waits until the default balance exists for the given account ID.
// It no longer attempts to create the default, as the system creates it asynchronously upon account creation.
func EnsureDefaultBalanceRecord(ctx context.Context, trans *HTTPClient, orgID, ledgerID, accountID string, headers map[string]string) error {
    deadline := time.Now().Add(10 * time.Second)
    for {
        c, b, e := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID), headers, nil)
        if e == nil && c == 200 {
            var paged struct{ Items []balanceItem `json:"items"` }
            _ = json.Unmarshal(b, &paged)
            for _, it := range paged.Items {
                if it.Key == "default" {
                    return nil
                }
            }
        }
        if time.Now().After(deadline) {
            return fmt.Errorf("default balance not ready for account %s", accountID)
        }
        time.Sleep(150 * time.Millisecond)
    }
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

// GetAvailableSumByAlias returns the sum of Available across all balances for the given alias and asset code.
func GetAvailableSumByAlias(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias, asset string, headers map[string]string) (decimal.Decimal, error) {
    code, body, err := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", orgID, ledgerID, alias), headers, nil)
    if err != nil {
        return decimal.Zero, err
    }
    if code != 200 {
        return decimal.Zero, fmt.Errorf("balances by alias status=%d body=%s", code, string(body))
    }
    var paged struct{
        Items []struct{
            AssetCode string          `json:"assetCode"`
            Available decimal.Decimal `json:"available"`
        } `json:"items"`
    }
    if err := json.Unmarshal(body, &paged); err != nil {
        return decimal.Zero, err
    }
    sum := decimal.Zero
    for _, it := range paged.Items {
        if it.AssetCode == asset {
            sum = sum.Add(it.Available)
        }
    }
    return sum, nil
}

// WaitForAvailableSumByAlias polls until the available sum for alias equals expected, or timeout.
func WaitForAvailableSumByAlias(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias, asset string, headers map[string]string, expected decimal.Decimal, timeout time.Duration) (decimal.Decimal, error) {
    deadline := time.Now().Add(timeout)
    var last decimal.Decimal
    for {
        cur, err := GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, asset, headers)
        if err == nil {
            last = cur
            if cur.Equal(expected) {
                return cur, nil
            }
            // guard that it never becomes negative
            if cur.IsNegative() {
                return cur, fmt.Errorf("available for alias %s became negative: %s", alias, cur.String())
            }
        }
        if time.Now().After(deadline) {
            return last, fmt.Errorf("timeout waiting for available sum; last=%s expected=%s", last.String(), expected.String())
        }
        time.Sleep(150 * time.Millisecond)
    }
}
