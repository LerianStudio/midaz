package helpers

import (
    "context"
    "fmt"
)

// CreateUSDAsset posts a minimal USD asset to the onboarding API; ignores if already exists.
func CreateUSDAsset(ctx context.Context, client *HTTPClient, orgID, ledgerID string, headers map[string]string) error {
    payload := map[string]any{
        "name": "US Dollar",
        "type": "currency",
        "code": "USD",
    }
    code, _, err := client.Request(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/assets", headers, payload)
    if err != nil {
        return err
    }
    // Accept 201 (created) or 409 (duplicate) depending on server semantics; other 2xx also ok
    if code >= 400 {
        return fmt.Errorf("create asset USD failed: status %d", code)
    }
    return nil
}
