package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// ═══════════════════════════════════════════════════════════════════════════════
// BALANCE MUTATION TYPES
// ═══════════════════════════════════════════════════════════════════════════════

// balanceMutResponse represents the API response for a balance
type balanceMutResponse struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	LedgerID       string          `json:"ledgerId"`
	AccountID      string          `json:"accountId"`
	Alias          string          `json:"alias"`
	Key            string          `json:"key"`
	AssetCode      string          `json:"assetCode"`
	Available      decimal.Decimal `json:"available"`
	OnHold         decimal.Decimal `json:"onHold"`
	Version        int64           `json:"version"`
	AccountType    string          `json:"accountType"`
	AllowSending   bool            `json:"allowSending"`
	AllowReceiving bool            `json:"allowReceiving"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	Metadata       map[string]any  `json:"metadata"`
}

// balancesMutListResponse represents paginated balances
type balancesMutListResponse struct {
	Items []balanceMutResponse `json:"items"`
}

// ═══════════════════════════════════════════════════════════════════════════════
// BALANCE UPDATE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Balance_Update tests PATCH operation for balances.
// This test:
// 1. Creates org, ledger, USD asset, and an account
// 2. GET balances for account to find balance ID
// 3. PATCH balance to toggle allowSending and allowReceiving
// 4. Verifies update was applied
// 5. GET balance again to confirm persistence
func TestIntegration_Balance_Update(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup: organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("BalUpdate"))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// Setup: ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("BalUpdate"))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Setup: USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Setup: account
	alias := iso.UniqueAccountAlias("bal-update")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, alias)

	// GET balances for account to find the default balance ID
	listPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
	code, body, err := trans.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list balances failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var list balancesMutListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("parse balances list: %v body=%s", err, string(body))
	}

	var balanceID string
	var originalAllowSending, originalAllowReceiving bool
	for _, bal := range list.Items {
		if bal.Key == "default" {
			balanceID = bal.ID
			originalAllowSending = bal.AllowSending
			originalAllowReceiving = bal.AllowReceiving
			break
		}
	}

	if balanceID == "" {
		t.Fatalf("default balance not found in listing")
	}
	t.Logf("Found default balance: ID=%s AllowSending=%v AllowReceiving=%v",
		balanceID, originalAllowSending, originalAllowReceiving)

	// PATCH balance to toggle flags
	newAllowSending := !originalAllowSending
	newAllowReceiving := !originalAllowReceiving
	updatePayload := map[string]any{
		"allowSending":   newAllowSending,
		"allowReceiving": newAllowReceiving,
		"metadata": map[string]any{
			"updated":     true,
			"environment": "integration-test",
		},
	}

	patchPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, balanceID)
	code, body, err = trans.Request(ctx, "PATCH", patchPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH balance failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedBalance balanceMutResponse
	if err := json.Unmarshal(body, &updatedBalance); err != nil {
		t.Fatalf("parse updated balance: %v body=%s", err, string(body))
	}

	// Verify update was applied
	if updatedBalance.AllowSending != newAllowSending {
		t.Errorf("allowSending not updated: got %v, want %v", updatedBalance.AllowSending, newAllowSending)
	}
	if updatedBalance.AllowReceiving != newAllowReceiving {
		t.Errorf("allowReceiving not updated: got %v, want %v", updatedBalance.AllowReceiving, newAllowReceiving)
	}
	t.Logf("Updated balance: ID=%s AllowSending=%v AllowReceiving=%v",
		updatedBalance.ID, updatedBalance.AllowSending, updatedBalance.AllowReceiving)

	// GET balance again to confirm persistence
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, balanceID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET balance after update failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var verifyBalance balanceMutResponse
	if err := json.Unmarshal(body, &verifyBalance); err != nil {
		t.Fatalf("parse verify balance: %v body=%s", err, string(body))
	}

	if verifyBalance.AllowSending != newAllowSending {
		t.Errorf("allowSending not persisted: got %v, want %v", verifyBalance.AllowSending, newAllowSending)
	}
	if verifyBalance.AllowReceiving != newAllowReceiving {
		t.Errorf("allowReceiving not persisted: got %v, want %v", verifyBalance.AllowReceiving, newAllowReceiving)
	}

	t.Log("Balance PATCH test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// BALANCE CREATION TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Balance_CreateAdditional tests creating additional balances for an account.
// This test:
// 1. Creates setup (org, ledger, asset, account)
// 2. POST additional balance with custom key (e.g., "escrow-xxx")
// 3. Verifies the created balance has the correct key
// 4. Lists balances to confirm there are now 2
// 5. Tests duplicate key returns 409
func TestIntegration_Balance_CreateAdditional(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup: organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("BalCreate"))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// Setup: ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("BalCreate"))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Setup: USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Setup: account
	alias := iso.UniqueAccountAlias("bal-create")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, alias)

	// Verify initial balance count (should be 1 - the default)
	listPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
	code, body, err := trans.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list balances (initial) failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var initialList balancesMutListResponse
	if err := json.Unmarshal(body, &initialList); err != nil {
		t.Fatalf("parse initial balances list: %v body=%s", err, string(body))
	}

	initialCount := len(initialList.Items)
	t.Logf("Initial balance count: %d", initialCount)

	// POST additional balance with custom key
	customKey := fmt.Sprintf("escrow-%s", h.RandString(6))
	createPayload := map[string]any{
		"key": customKey,
		"metadata": map[string]any{
			"purpose": "escrow",
			"created": "integration-test",
		},
	}

	createPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
	code, body, err = trans.Request(ctx, "POST", createPath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("POST additional balance failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdBalance balanceMutResponse
	if err := json.Unmarshal(body, &createdBalance); err != nil {
		t.Fatalf("parse created balance: %v body=%s", err, string(body))
	}

	// Verify the created balance has the correct key
	if createdBalance.Key != customKey {
		t.Errorf("created balance key mismatch: got %q, want %q", createdBalance.Key, customKey)
	}
	if createdBalance.AccountID != accountID {
		t.Errorf("created balance accountId mismatch: got %q, want %q", createdBalance.AccountID, accountID)
	}
	if createdBalance.AssetCode != "USD" {
		t.Errorf("created balance assetCode mismatch: got %q, want %q", createdBalance.AssetCode, "USD")
	}
	t.Logf("Created additional balance: ID=%s Key=%s AccountID=%s",
		createdBalance.ID, createdBalance.Key, createdBalance.AccountID)

	// List balances to confirm there are now 2
	code, body, err = trans.Request(ctx, "GET", listPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list balances (after create) failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var afterCreateList balancesMutListResponse
	if err := json.Unmarshal(body, &afterCreateList); err != nil {
		t.Fatalf("parse after-create balances list: %v body=%s", err, string(body))
	}

	expectedCount := initialCount + 1
	if len(afterCreateList.Items) != expectedCount {
		t.Errorf("balance count after create: got %d, want %d", len(afterCreateList.Items), expectedCount)
	}

	// Verify our custom key exists in the list
	foundCustomKey := false
	for _, bal := range afterCreateList.Items {
		if bal.Key == customKey {
			foundCustomKey = true
			break
		}
	}
	if !foundCustomKey {
		t.Errorf("custom key %q not found in balance list", customKey)
	}
	t.Logf("After create balance count: %d (found custom key: %v)", len(afterCreateList.Items), foundCustomKey)

	// Test duplicate key returns 409
	code, body, err = trans.Request(ctx, "POST", createPath, headers, createPayload)
	if err != nil {
		t.Logf("Duplicate create request error (may be expected): %v", err)
	}

	if code != 409 {
		t.Errorf("duplicate balance key should return 409, got %d: body=%s", code, string(body))
	} else {
		t.Logf("Duplicate balance key correctly rejected with 409")
	}

	t.Log("Balance CreateAdditional test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// BALANCE DELETE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Balance_Delete tests DELETE operation for balances.
// This test:
// 1. Creates setup and an additional balance
// 2. DELETE the additional balance
// 3. Verifies GET returns 404 after delete
func TestIntegration_Balance_Delete(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup: organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("BalDelete"))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// Setup: ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("BalDelete"))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Setup: USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Setup: account
	alias := iso.UniqueAccountAlias("bal-delete")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, alias)

	// Create additional balance to delete
	customKey := fmt.Sprintf("delete-%s", h.RandString(6))
	createPayload := map[string]any{
		"key": customKey,
		"metadata": map[string]any{
			"purpose": "to-be-deleted",
		},
	}

	createPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", orgID, ledgerID, accountID)
	code, body, err := trans.Request(ctx, "POST", createPath, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("POST additional balance failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdBalance balanceMutResponse
	if err := json.Unmarshal(body, &createdBalance); err != nil {
		t.Fatalf("parse created balance: %v body=%s", err, string(body))
	}
	balanceID := createdBalance.ID
	t.Logf("Created balance for deletion: ID=%s Key=%s", balanceID, customKey)

	// Verify balance exists before delete
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, balanceID)
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET balance before delete failed: code=%d err=%v body=%s", code, err, string(body))
	}
	t.Logf("Verified balance exists before delete")

	// DELETE the balance
	deletePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", orgID, ledgerID, balanceID)
	code, body, err = trans.Request(ctx, "DELETE", deletePath, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE balance failed: code=%d err=%v body=%s", code, err, string(body))
	}
	t.Logf("Deleted balance: ID=%s (code=%d)", balanceID, code)

	// Verify GET returns 404 after delete
	code, body, err = trans.Request(ctx, "GET", getPath, headers, nil)
	if err != nil {
		t.Logf("GET after delete error (may be expected): %v", err)
	}

	if code != 404 {
		t.Errorf("expected 404 after delete, got %d: body=%s", code, string(body))
	} else {
		t.Logf("Balance correctly returns 404 after delete")
	}

	t.Log("Balance DELETE test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// BALANCE GET BY EXTERNAL CODE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Balance_GetByExternalCode tests GET balances by account external code.
// This test:
// 1. Creates account with externalCode
// 2. GET balances by external code
// 3. Verifies balances returned
func TestIntegration_Balance_GetByExternalCode(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup: organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("BalExtCode"))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// Setup: ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("BalExtCode"))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Setup: USD asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Setup: account with external code
	alias := iso.UniqueAccountAlias("bal-extcode")
	externalCode := fmt.Sprintf("EXT-%s", h.RandString(8))

	// Create account with externalCode
	accPayload := map[string]any{
		"name":         "External Code Test Account",
		"assetCode":    "USD",
		"type":         "deposit",
		"alias":        alias,
		"externalCode": externalCode,
	}

	accPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", accPath, headers, accPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account with externalCode failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdAccount struct {
		ID           string `json:"id"`
		ExternalCode string `json:"externalCode"`
	}
	if err := json.Unmarshal(body, &createdAccount); err != nil {
		t.Fatalf("parse created account: %v body=%s", err, string(body))
	}
	accountID := createdAccount.ID
	t.Logf("Created account: ID=%s ExternalCode=%s", accountID, externalCode)

	// GET balances by external code
	extCodePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/external/%s/balances",
		orgID, ledgerID, url.PathEscape(externalCode))

	// Allow some time for balance creation to propagate
	var balancesList balancesMutListResponse
	deadline := time.Now().Add(10 * time.Second)
	for {
		code, body, err = trans.Request(ctx, "GET", extCodePath, headers, nil)
		if err == nil && code == 200 {
			if err := json.Unmarshal(body, &balancesList); err == nil && len(balancesList.Items) > 0 {
				break
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for balances by external code: code=%d err=%v body=%s", code, err, string(body))
		}
		time.Sleep(150 * time.Millisecond)
	}

	// Verify balances returned
	if len(balancesList.Items) == 0 {
		t.Fatalf("no balances returned for external code %s", externalCode)
	}

	// Verify the returned balances belong to our account
	for _, bal := range balancesList.Items {
		if bal.AccountID != accountID {
			t.Errorf("balance accountId mismatch: got %q, want %q", bal.AccountID, accountID)
		}
		if bal.Alias != alias {
			t.Errorf("balance alias mismatch: got %q, want %q", bal.Alias, alias)
		}
	}

	t.Logf("Found %d balances for external code %s", len(balancesList.Items), externalCode)

	// Verify default balance exists
	foundDefault := false
	for _, bal := range balancesList.Items {
		if bal.Key == "default" {
			foundDefault = true
			t.Logf("Found default balance: ID=%s AssetCode=%s", bal.ID, bal.AssetCode)
			break
		}
	}
	if !foundDefault {
		t.Errorf("default balance not found in response")
	}

	// Test 404 for non-existent external code
	nonExistentPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/external/%s/balances",
		orgID, ledgerID, url.PathEscape("NON-EXISTENT-CODE-"+h.RandString(8)))

	code, body, err = trans.Request(ctx, "GET", nonExistentPath, headers, nil)
	if err != nil {
		t.Logf("GET non-existent external code error (may be expected): %v", err)
	}

	if code != 404 {
		t.Errorf("expected 404 for non-existent external code, got %d: body=%s", code, string(body))
	} else {
		t.Logf("Non-existent external code correctly returned 404")
	}

	t.Log("Balance GetByExternalCode test completed successfully")
}
