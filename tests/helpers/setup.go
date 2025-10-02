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

// SetupInflow posts a simple inflow transaction to credit an alias with amount for a given asset code.
// Returns status code and body for assertion when needed.
func SetupInflowTransaction(ctx context.Context, trans *HTTPClient, orgID, ledgerID, alias, assetCode, amount string, headers map[string]string) (int, []byte, error) {
	payload := map[string]any{
		"send": map[string]any{
			"asset": assetCode,
			"value": amount,
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": assetCode, "value": amount},
				}},
			},
		},
	}

	path := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID + "/transactions/inflow"
	code, body, err := trans.Request(ctx, "POST", path, headers, payload)
	return code, body, err
}

// CreateOrganization creates an organization and returns its ID.
func SetupOrganization(ctx context.Context, onboard *HTTPClient, headers map[string]string, name string) (string, error) {
	payload := OrgPayload(name, RandString(12))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil || org.ID == "" {
		return "", fmt.Errorf("parse organization: %v body=%s", err, string(body))
	}

	return org.ID, nil
}

// CreateLedger creates a ledger under the given organization and returns its ID.
func SetupLedger(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, name string) (string, error) {
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers", headers, map[string]any{"name": name})
	if err != nil || code != 201 {
		return "", fmt.Errorf("create ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil || ledger.ID == "" {
		return "", fmt.Errorf("parse ledger: %v body=%s", err, string(body))
	}

	return ledger.ID, nil
}

// CreateAccount creates an account with alias and asset code (type=deposit) and returns its ID.
func SetupAccount(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, alias, assetCode string) (string, error) {
	payload := map[string]any{
		"name":      "Test Account",
		"assetCode": assetCode,
		"type":      "deposit",
		"alias":     alias,
	}

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/accounts", headers, payload)
	if err != nil || code != 201 {
		return "", fmt.Errorf("create account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var account struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &account); err != nil || account.ID == "" {
		return "", fmt.Errorf("parse account: %v body=%s", err, string(body))
	}

	return account.ID, nil
}
