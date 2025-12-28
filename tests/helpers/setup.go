package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	setupAssetRetryAttempts   = 4
	setupAssetRetryBackoff    = 250 * time.Millisecond
	setupAssetPollTimeout     = 12 * time.Second
	setupAssetPollInterval    = 150 * time.Millisecond
	setupHTTPStatusOK         = 200
	setupHTTPStatusCreated    = 201
	setupHTTPStatusBadRequest = 400
	setupHTTPStatusConflict   = 409
	setupRandStringLength     = 12
)

// ErrAssetCreationFailed indicates asset creation failed
var ErrAssetCreationFailed = errors.New("create asset USD failed")

// CreateUSDAsset posts a minimal USD asset to the onboarding API; ignores if already exists.
func CreateUSDAsset(ctx context.Context, client *HTTPClient, orgID, ledgerID string, headers map[string]string) error {
	if err := createAssetRequest(ctx, client, orgID, ledgerID, headers); err != nil {
		return err
	}

	return waitForAssetAvailable(ctx, client, orgID, ledgerID, headers)
}

// createAssetRequest sends the asset creation request
func createAssetRequest(ctx context.Context, client *HTTPClient, orgID, ledgerID string, headers map[string]string) error {
	payload := map[string]any{
		"name": "US Dollar",
		"type": "currency",
		"code": "USD",
	}

	// Use retry to handle transient restart windows (e.g., rolling restarts/redis blips)
	code, body, _, err := client.RequestFullWithRetry(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/assets", headers, payload, setupAssetRetryAttempts, setupAssetRetryBackoff)
	if err != nil {
		return err
	}
	// Accept 201 (created) or 409 (duplicate) depending on server semantics; other 2xx also ok
	if code >= setupHTTPStatusBadRequest && code != setupHTTPStatusConflict {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("%w: status %d body=%s", ErrAssetCreationFailed, code, string(body))
	}

	return nil
}

// waitForAssetAvailable polls until the asset appears in the listing
func waitForAssetAvailable(ctx context.Context, client *HTTPClient, orgID, ledgerID string, headers map[string]string) error {
	deadline := time.Now().Add(setupAssetPollTimeout)

	for {
		c, b, e := client.Request(ctx, "GET", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/assets", headers, nil)
		if e == nil && c == setupHTTPStatusOK {
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

		time.Sleep(setupAssetPollInterval)
	}

	return nil
}

// SetupInflowTransaction posts a simple inflow transaction to credit an alias with amount for a given asset code.
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

// SetupOrganization creates an organization and returns its ID.
func SetupOrganization(ctx context.Context, onboard *HTTPClient, headers map[string]string, name string) (string, error) {
	payload := OrgPayload(name, RandString(setupRandStringLength))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, payload)
	if err != nil || code != setupHTTPStatusCreated {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("create organization failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil || org.ID == "" {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("parse organization: %w body=%s", err, string(body))
	}

	return org.ID, nil
}

// SetupLedger creates a ledger under the given organization and returns its ID.
func SetupLedger(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, name string) (string, error) {
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers", headers, map[string]any{"name": name})
	if err != nil || code != setupHTTPStatusCreated {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("create ledger failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil || ledger.ID == "" {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("parse ledger: %w body=%s", err, string(body))
	}

	return ledger.ID, nil
}

// SetupAccount creates an account with alias and asset code (type=deposit) and returns its ID.
func SetupAccount(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, alias, assetCode string) (string, error) {
	payload := map[string]any{
		"name":      "Test Account",
		"assetCode": assetCode,
		"type":      "deposit",
		"alias":     alias,
	}

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations/"+orgID+"/ledgers/"+ledgerID+"/accounts", headers, payload)
	if err != nil || code != setupHTTPStatusCreated {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("create account failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var account struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &account); err != nil || account.ID == "" {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("parse account: %w body=%s", err, string(body))
	}

	return account.ID, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// ACCOUNT TYPE HELPERS
// ═══════════════════════════════════════════════════════════════════════════════

// AccountTypeResponse represents an account type API response.
type AccountTypeResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	KeyValue       string         `json:"keyValue"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

// AccountTypeListResponse represents the account type list API response.
type AccountTypeListResponse struct {
	Items []AccountTypeResponse `json:"items"`
}

// CreateAccountTypePayload returns a valid account type creation payload.
func CreateAccountTypePayload(name, keyValue string) map[string]any {
	return map[string]any{
		"name":     name,
		"keyValue": keyValue,
	}
}

// SetupAccountType creates an account type and returns its ID.
func SetupAccountType(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, name, keyValue string) (string, error) {
	payload := CreateAccountTypePayload(name, keyValue)
	path := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID + "/account-types"

	code, body, err := onboard.Request(ctx, "POST", path, headers, payload)
	if err != nil || code != setupHTTPStatusCreated {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("create account type failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var accountType AccountTypeResponse
	if err := json.Unmarshal(body, &accountType); err != nil || accountType.ID == "" {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return "", fmt.Errorf("parse account type: %w body=%s", err, string(body))
	}

	return accountType.ID, nil
}

// GetAccountType retrieves an account type by ID.
func GetAccountType(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, accountTypeID string) (*AccountTypeResponse, error) {
	path := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID + "/account-types/" + accountTypeID
	code, body, err := onboard.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != setupHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("get account type failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var accountType AccountTypeResponse
	if err := json.Unmarshal(body, &accountType); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse account type: %w body=%s", err, string(body))
	}

	return &accountType, nil
}

// ListAccountTypes retrieves all account types for a ledger.
func ListAccountTypes(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID string) (*AccountTypeListResponse, error) {
	path := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID + "/account-types"
	code, body, err := onboard.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != setupHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("list account types failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var list AccountTypeListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse account types list: %w body=%s", err, string(body))
	}

	return &list, nil
}

// UpdateAccountType updates an account type and returns the updated type.
func UpdateAccountType(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, accountTypeID string, payload map[string]any) (*AccountTypeResponse, error) {
	path := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID + "/account-types/" + accountTypeID
	code, body, err := onboard.Request(ctx, "PATCH", path, headers, payload)
	if err != nil || code != setupHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("update account type failed: code=%d err=%w body=%s", code, err, string(body))
	}

	var accountType AccountTypeResponse
	if err := json.Unmarshal(body, &accountType); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("parse account type: %w body=%s", err, string(body))
	}

	return &accountType, nil
}

// DeleteAccountType deletes an account type by ID.
func DeleteAccountType(ctx context.Context, onboard *HTTPClient, headers map[string]string, orgID, ledgerID, accountTypeID string) error {
	path := "/v1/organizations/" + orgID + "/ledgers/" + ledgerID + "/account-types/" + accountTypeID
	code, body, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("delete account type failed: code=%d err=%w body=%s", code, err, string(body))
	}

	return nil
}
