package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_AccountType_CRUDLifecycle tests the complete CRUD lifecycle for Account Types.
// Account types define categories of accounts (e.g., checking, savings, investment).
func TestIntegration_AccountType_CRUDLifecycle(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AcctType Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AcctType Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Test data
	typeName := fmt.Sprintf("Checking Account %s", h.RandString(6))
	keyValue := fmt.Sprintf("checking_%s", h.RandString(6))
	updatedName := fmt.Sprintf("Updated Checking %s", h.RandString(6))

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - Account Type
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := h.CreateAccountTypePayload(typeName, keyValue)
	createPayload["description"] = "Test account type for checking accounts"
	createPayload["metadata"] = map[string]any{"environment": "test", "category": "deposit"}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE account type failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdType h.AccountTypeResponse
	if err := json.Unmarshal(body, &createdType); err != nil || createdType.ID == "" {
		t.Fatalf("parse created account type: %v body=%s", err, string(body))
	}

	accountTypeID := createdType.ID
	t.Logf("Created account type: ID=%s Name=%s KeyValue=%s",
		accountTypeID, createdType.Name, createdType.KeyValue)

	// Verify created account type fields
	if createdType.Name != typeName {
		t.Errorf("account type name mismatch: got %q, want %q", createdType.Name, typeName)
	}
	if createdType.KeyValue != strings.ToLower(keyValue) {
		t.Errorf("account type keyValue mismatch: got %q, want %q", createdType.KeyValue, strings.ToLower(keyValue))
	}
	if createdType.OrganizationID != orgID {
		t.Errorf("account type organization ID mismatch: got %q, want %q", createdType.OrganizationID, orgID)
	}
	if createdType.LedgerID != ledgerID {
		t.Errorf("account type ledger ID mismatch: got %q, want %q", createdType.LedgerID, ledgerID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - Get Account Type by ID
	// ─────────────────────────────────────────────────────────────────────────
	fetchedType, err := h.GetAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID)
	if err != nil {
		t.Fatalf("GET account type by ID failed: %v", err)
	}

	if fetchedType.ID != accountTypeID {
		t.Errorf("fetched type ID mismatch: got %q, want %q", fetchedType.ID, accountTypeID)
	}
	if fetchedType.Name != typeName {
		t.Errorf("fetched type name mismatch: got %q, want %q", fetchedType.Name, typeName)
	}

	t.Logf("Fetched account type: ID=%s Name=%s", fetchedType.ID, fetchedType.Name)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) LIST - Get All Account Types (verify our type appears)
	// ─────────────────────────────────────────────────────────────────────────
	typeList, err := h.ListAccountTypes(ctx, onboard, headers, orgID, ledgerID)
	if err != nil {
		t.Fatalf("LIST account types failed: %v", err)
	}

	found := false
	for _, at := range typeList.Items {
		if at.ID == accountTypeID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created account type not found in list: ID=%s", accountTypeID)
	}

	t.Logf("List account types: found %d types, target type found=%v", len(typeList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) UPDATE - Modify Account Type Name (keyValue is immutable)
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"name":        updatedName,
		"description": "Updated description for integration testing",
		"metadata": map[string]any{
			"environment": "test",
			"category":    "deposit",
			"updated":     true,
		},
	}

	updatedType, err := h.UpdateAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID, updatePayload)
	if err != nil {
		t.Fatalf("UPDATE account type failed: %v", err)
	}

	if updatedType.Name != updatedName {
		t.Errorf("updated type name mismatch: got %q, want %q", updatedType.Name, updatedName)
	}
	// KeyValue should remain unchanged (immutable, stored as lowercase)
	if updatedType.KeyValue != strings.ToLower(keyValue) {
		t.Errorf("updated type keyValue should not change: got %q, want %q", updatedType.KeyValue, strings.ToLower(keyValue))
	}

	t.Logf("Updated account type: ID=%s NewName=%s", updatedType.ID, updatedType.Name)

	// Verify update persisted by fetching again
	verifyType, err := h.GetAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID)
	if err != nil {
		t.Fatalf("GET account type after update failed: %v", err)
	}
	if verifyType.Name != updatedName {
		t.Errorf("persisted name mismatch: got %q, want %q", verifyType.Name, updatedName)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 5) DELETE - Remove Account Type
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID)
	if err != nil {
		t.Fatalf("DELETE account type failed: %v", err)
	}

	t.Logf("Deleted account type: ID=%s", accountTypeID)

	// Verify deletion - GET should fail
	_, err = h.GetAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID)
	if err == nil {
		t.Errorf("GET deleted account type should fail, but succeeded")
	}

	t.Log("Account Type CRUD lifecycle completed successfully")
}

// TestIntegration_AccountType_MultipleTypes tests creating multiple account types in the same ledger.
func TestIntegration_AccountType_MultipleTypes(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("MultiType Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("MultiType Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create multiple account types
	types := []struct {
		name     string
		keyValue string
	}{
		{"Checking Account", fmt.Sprintf("checking_%s", h.RandString(4))},
		{"Savings Account", fmt.Sprintf("savings_%s", h.RandString(4))},
		{"Investment Account", fmt.Sprintf("investment_%s", h.RandString(4))},
		{"Credit Card", fmt.Sprintf("credit_%s", h.RandString(4))},
	}

	createdIDs := make([]string, 0, len(types))
	for _, at := range types {
		typeID, err := h.SetupAccountType(ctx, onboard, headers, orgID, ledgerID, at.name, at.keyValue)
		if err != nil {
			t.Fatalf("create %s type failed: %v", at.name, err)
		}
		createdIDs = append(createdIDs, typeID)
		t.Logf("Created account type: ID=%s Name=%s KeyValue=%s", typeID, at.name, at.keyValue)

		// Register cleanup for each created type
		typeIDToCleanup := typeID // capture for closure
		t.Cleanup(func() {
			if err := h.DeleteAccountType(ctx, onboard, headers, orgID, ledgerID, typeIDToCleanup); err != nil {
				t.Logf("Warning: cleanup delete account type failed: %v", err)
			}
		})
	}

	// List all account types
	typeList, err := h.ListAccountTypes(ctx, onboard, headers, orgID, ledgerID)
	if err != nil {
		t.Fatalf("list account types failed: %v", err)
	}

	if len(typeList.Items) < len(types) {
		t.Errorf("expected at least %d account types, got %d", len(types), len(typeList.Items))
	}

	// Verify all created types exist
	foundCount := 0
	for _, id := range createdIDs {
		for _, at := range typeList.Items {
			if at.ID == id {
				foundCount++
				break
			}
		}
	}

	if foundCount != len(createdIDs) {
		t.Errorf("not all created types found in list: found %d, expected %d", foundCount, len(createdIDs))
	}

	t.Logf("Multiple account types test passed: created %d types, found %d in list", len(createdIDs), foundCount)
}

// TestIntegration_AccountType_DuplicateKeyValue tests that duplicate keyValues are rejected.
func TestIntegration_AccountType_DuplicateKeyValue(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("DupKey Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("DupKey Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create first account type
	sharedKeyValue := fmt.Sprintf("shared_key_%s", h.RandString(6))
	type1ID, err := h.SetupAccountType(ctx, onboard, headers, orgID, ledgerID, "First Type", sharedKeyValue)
	if err != nil {
		t.Fatalf("create first account type failed: %v", err)
	}
	t.Logf("Created first account type: ID=%s KeyValue=%s", type1ID, sharedKeyValue)

	// Register cleanup for first type
	t.Cleanup(func() {
		if err := h.DeleteAccountType(ctx, onboard, headers, orgID, ledgerID, type1ID); err != nil {
			t.Logf("Warning: cleanup delete account type failed: %v", err)
		}
	})

	// Attempt to create second account type with SAME keyValue - should fail
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)
	duplicatePayload := map[string]any{
		"name":     "Second Type",
		"keyValue": sharedKeyValue, // Same key!
	}

	code, body, err := onboard.Request(ctx, "POST", path, headers, duplicatePayload)
	if err != nil {
		t.Logf("Request error (may be expected for duplicate): %v", err)
	}

	// Should get 409 Conflict or 400 Bad Request for duplicate keyValue
	if code == 201 {
		t.Errorf("duplicate keyValue should be rejected, but got 201 Created: body=%s", string(body))
		// Cleanup the accidentally created type
		var type2 h.AccountTypeResponse
		if json.Unmarshal(body, &type2) == nil && type2.ID != "" {
			if delErr := h.DeleteAccountType(ctx, onboard, headers, orgID, ledgerID, type2.ID); delErr != nil {
				t.Logf("Warning: cleanup delete accidental account type failed: %v", delErr)
			}
		}
	} else {
		t.Logf("Duplicate keyValue correctly rejected: code=%d", code)
	}

	t.Log("Duplicate keyValue validation test completed")
}

// TestIntegration_AccountType_Validation tests validation errors for account type creation.
func TestIntegration_AccountType_Validation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Validation Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Validation Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "missing name",
			payload: map[string]any{"keyValue": "valid_key"},
		},
		{
			name:    "missing keyValue",
			payload: map[string]any{"name": "Valid Name"},
		},
		{
			name:    "name exceeds max (101 chars, max is 100)",
			payload: map[string]any{"name": h.RandString(101), "keyValue": "valid_key"},
		},
		{
			name:    "keyValue exceeds max (51 chars, max is 50)",
			payload: map[string]any{"name": "Valid Name", "keyValue": h.RandString(51)},
		},
		{
			name:    "description exceeds max (501 chars, max is 500)",
			payload: map[string]any{"name": "Valid Name", "keyValue": "valid_key", "description": h.RandString(501)},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := onboard.Request(ctx, "POST", path, headers, tc.payload)
			if err != nil {
				t.Logf("Request error (expected for validation): %v", err)
			}
			// Expect 400 Bad Request for validation errors
			if code != 400 {
				t.Errorf("expected 400 Bad Request for %s, but got %d: body=%s", tc.name, code, string(body))
			}
			t.Logf("Validation test %s: code=%d (expected 400)", tc.name, code)
		})
	}
}
