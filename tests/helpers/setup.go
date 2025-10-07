// Package helpers provides test utilities and helper functions for integration tests.
// This file contains test setup and teardown utilities.
package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// CreateUSDAsset posts a minimal USD asset to the onboarding API; ignores if already exists.
func CreateUSDAsset(ctx context.Context, client *HTTPClient, orgID, ledgerID string, headers map[string]string) error {
	payload := map[string]any{
		"name": "US Dollar",
		"type": "currency",
		"code": "USD",
	}

	// Use retry to handle transient restart windows (e.g., rolling restarts/redis blips)
	code, body, _, err := client.RequestFullWithRetry(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/assets", headers, payload, 4, 250*time.Millisecond)
	if err != nil {
		return err
	}
	// Accept 201 (created) or 409 (duplicate) depending on server semantics; other 2xx also ok
	if code >= 400 && code != 409 {
		return fmt.Errorf("create asset USD failed: status %d body=%s", code, string(body))
	}

	// Poll until asset appears in listing to avoid race with subsequent account creation
	deadline := time.Now().Add(12 * time.Second)
	for {
		c, b, e := client.Request(ctx, "GET", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/assets", headers, nil)
		if e == nil && c == 200 {
			var list struct {
				Items []struct {
					Code string `json:"code"`
				} `json:"items"`
			}
			_ = json.Unmarshal(b, &list)
			found := false
			for _, it := range list.Items {
				if it.Code == "USD" {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	return nil
}
