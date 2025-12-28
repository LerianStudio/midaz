package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// operationResponse represents the API response for an operation
type operationResponse struct {
	ID             string         `json:"id"`
	TransactionID  string         `json:"transactionId"`
	Description    string         `json:"description"`
	Type           string         `json:"type"`
	AssetCode      string         `json:"assetCode"`
	AccountID      string         `json:"accountId"`
	AccountAlias   string         `json:"accountAlias"`
	BalanceID      string         `json:"balanceId"`
	OrganizationID string         `json:"organizationId"`
	LedgerID       string         `json:"ledgerId"`
	Amount         map[string]any `json:"amount"`
	Balance        map[string]any `json:"balance"`
	BalanceAfter   map[string]any `json:"balanceAfter"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// operationsListResponse represents paginated operations
type operationsListResponse struct {
	Items []operationResponse `json:"items"`
}

// TestIntegration_Operations_GetByAccount tests retrieving operations by account ID.
// This test verifies:
// 1. Setup - Create org, ledger, USD asset, and an account
// 2. Create an inflow transaction to generate operations
// 3. GET operations by account ID
// 4. Verify at least 1 operation exists
// 5. Verify operations belong to the correct account
func TestIntegration_Operations_GetByAccount(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Asset → Account
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Operations GetByAccount Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Operations GetByAccount Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	accountAlias := fmt.Sprintf("ops-test-account-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, accountAlias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, accountAlias)

	// ─────────────────────────────────────────────────────────────────────────
	// Create an inflow transaction to generate operations
	// ─────────────────────────────────────────────────────────────────────────
	inflowAmount := "1000"
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, accountAlias, "USD", inflowAmount, headers)
	if err != nil || (code != 200 && code != 201) {
		t.Fatalf("create inflow transaction failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var txnResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &txnResp); err != nil {
		t.Fatalf("parse transaction response: %v body=%s", err, string(body))
	}
	t.Logf("Created inflow transaction: ID=%s Amount=%s USD", txnResp.ID, inflowAmount)

	// Allow time for async processing
	time.Sleep(500 * time.Millisecond)

	// ─────────────────────────────────────────────────────────────────────────
	// GET operations by account ID
	// ─────────────────────────────────────────────────────────────────────────
	operationsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations", orgID, ledgerID, accountID)
	code, body, err = trans.Request(ctx, "GET", operationsPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET operations by account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var opsList operationsListResponse
	if err := json.Unmarshal(body, &opsList); err != nil {
		t.Fatalf("parse operations list: %v body=%s", err, string(body))
	}

	t.Logf("Retrieved %d operations for account %s", len(opsList.Items), accountID)

	// ─────────────────────────────────────────────────────────────────────────
	// Verify at least 1 operation exists
	// ─────────────────────────────────────────────────────────────────────────
	if len(opsList.Items) == 0 {
		t.Fatalf("expected at least 1 operation, got 0")
	}

	// ─────────────────────────────────────────────────────────────────────────
	// Verify operations belong to the correct account
	// ─────────────────────────────────────────────────────────────────────────
	for i, op := range opsList.Items {
		if op.AccountID != accountID && op.AccountAlias != accountAlias {
			t.Errorf("operation %d does not belong to account: got accountID=%s accountAlias=%s, want accountID=%s or alias=%s",
				i, op.AccountID, op.AccountAlias, accountID, accountAlias)
		}

		// Log operation details
		t.Logf("Operation %d: ID=%s Type=%s AssetCode=%s TransactionID=%s",
			i, op.ID, op.Type, op.AssetCode, op.TransactionID)
	}

	// Verify the operation is linked to our transaction
	foundTxn := false
	for _, op := range opsList.Items {
		if op.TransactionID == txnResp.ID {
			foundTxn = true
			break
		}
	}
	if !foundTxn {
		t.Errorf("no operation found for transaction %s", txnResp.ID)
	}

	t.Log("Operations GetByAccount test completed successfully")
}

// TestIntegration_Operations_GetSingleAndUpdate tests retrieving and updating a single operation.
// This test verifies:
// 1. Setup - Create org, ledger, asset, account, and transaction
// 2. List operations to get an operation ID
// 3. GET single operation by account and operation ID
// 4. PATCH operation to update description and metadata
// 5. Verify update was persisted
func TestIntegration_Operations_GetSingleAndUpdate(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Asset → Account → Transaction
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Operations Update Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Operations Update Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	accountAlias := fmt.Sprintf("ops-update-account-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, accountAlias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, accountAlias)

	// Create an inflow transaction to generate operations
	inflowAmount := "500"
	code, body, err := h.SetupInflowTransaction(ctx, trans, orgID, ledgerID, accountAlias, "USD", inflowAmount, headers)
	if err != nil || (code != 200 && code != 201) {
		t.Fatalf("create inflow transaction failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var txnResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &txnResp); err != nil {
		t.Fatalf("parse transaction response: %v body=%s", err, string(body))
	}
	transactionID := txnResp.ID
	t.Logf("Created inflow transaction: ID=%s Amount=%s USD", transactionID, inflowAmount)

	// Allow time for async processing
	time.Sleep(500 * time.Millisecond)

	// ─────────────────────────────────────────────────────────────────────────
	// List operations to get an operation ID
	// ─────────────────────────────────────────────────────────────────────────
	operationsPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations", orgID, ledgerID, accountID)
	code, body, err = trans.Request(ctx, "GET", operationsPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET operations by account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var opsList operationsListResponse
	if err := json.Unmarshal(body, &opsList); err != nil {
		t.Fatalf("parse operations list: %v body=%s", err, string(body))
	}

	if len(opsList.Items) == 0 {
		t.Fatalf("expected at least 1 operation, got 0")
	}

	operationID := opsList.Items[0].ID
	t.Logf("Found operation: ID=%s", operationID)

	// ─────────────────────────────────────────────────────────────────────────
	// GET single operation by account and operation ID
	// ─────────────────────────────────────────────────────────────────────────
	singleOpPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/operations/%s", orgID, ledgerID, accountID, operationID)
	code, body, err = trans.Request(ctx, "GET", singleOpPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET single operation failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var fetchedOp operationResponse
	if err := json.Unmarshal(body, &fetchedOp); err != nil {
		t.Fatalf("parse fetched operation: %v body=%s", err, string(body))
	}

	if fetchedOp.ID != operationID {
		t.Errorf("fetched operation ID mismatch: got %q, want %q", fetchedOp.ID, operationID)
	}

	t.Logf("Fetched single operation: ID=%s Type=%s AssetCode=%s", fetchedOp.ID, fetchedOp.Type, fetchedOp.AssetCode)

	// ─────────────────────────────────────────────────────────────────────────
	// PATCH operation to update description and metadata
	// ─────────────────────────────────────────────────────────────────────────
	updatedDescription := fmt.Sprintf("Updated operation description %s", h.RandString(6))
	updatePayload := map[string]any{
		"description": updatedDescription,
		"metadata": map[string]any{
			"environment": "integration-test",
			"updated":     true,
			"testRun":     h.RandString(8),
		},
	}

	// PATCH uses the transaction-based path
	patchPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s/operations/%s", orgID, ledgerID, transactionID, operationID)
	code, body, err = trans.Request(ctx, "PATCH", patchPath, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH operation failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedOp operationResponse
	if err := json.Unmarshal(body, &updatedOp); err != nil {
		t.Fatalf("parse updated operation: %v body=%s", err, string(body))
	}

	t.Logf("Updated operation: ID=%s NewDescription=%s", updatedOp.ID, updatedOp.Description)

	// Verify description was updated
	if updatedOp.Description != updatedDescription {
		t.Errorf("operation description not updated: got %q, want %q", updatedOp.Description, updatedDescription)
	}

	// Verify metadata was updated
	if updatedOp.Metadata == nil {
		t.Errorf("operation metadata should not be nil after update")
	} else {
		if updatedOp.Metadata["environment"] != "integration-test" {
			t.Errorf("metadata environment mismatch: got %v, want %q", updatedOp.Metadata["environment"], "integration-test")
		}
		if updatedOp.Metadata["updated"] != true {
			t.Errorf("metadata updated flag mismatch: got %v, want %v", updatedOp.Metadata["updated"], true)
		}
	}

	// ─────────────────────────────────────────────────────────────────────────
	// Verify update was persisted by fetching the operation again
	// ─────────────────────────────────────────────────────────────────────────
	code, body, err = trans.Request(ctx, "GET", singleOpPath, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET operation after update failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var verifyOp operationResponse
	if err := json.Unmarshal(body, &verifyOp); err != nil {
		t.Fatalf("parse verify operation: %v body=%s", err, string(body))
	}

	if verifyOp.Description != updatedDescription {
		t.Errorf("persisted description mismatch: got %q, want %q", verifyOp.Description, updatedDescription)
	}

	if verifyOp.Metadata == nil || verifyOp.Metadata["environment"] != "integration-test" {
		t.Errorf("persisted metadata not correct: got %v", verifyOp.Metadata)
	}

	t.Log("Operations GetSingleAndUpdate test completed successfully")
}
