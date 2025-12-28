package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_AccountTypes_CRUD tests the complete CRUD lifecycle for account types.
// This test verifies:
// 1. POST creates a new account type with name, description, and metadata
// 2. GET lists all account types and verifies the created one is in the list
// 3. PATCH updates the description
// 4. Verifies update was applied
// 5. DELETE removes the account type
// 6. Verifies GET returns 404 after delete
func TestIntegration_AccountTypes_CRUD(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization and Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AccountTypes CRUD Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AccountTypes CRUD Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Test data
	typeName := fmt.Sprintf("Savings Account %s", h.RandString(6))
	keyValue := fmt.Sprintf("savings_%s", h.RandString(6))
	description := "Test savings account type for integration testing"
	metadata := map[string]any{
		"category":    "deposit",
		"environment": "test",
		"tier":        "standard",
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - POST new account type
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := map[string]any{
		"name":        typeName,
		"keyValue":    keyValue,
		"description": description,
		"metadata":    metadata,
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	if code != http.StatusCreated {
		t.Fatalf("CREATE account type: expected %d, got %d, body=%s", http.StatusCreated, code, string(body))
	}

	var createdType h.AccountTypeResponse
	if err := json.Unmarshal(body, &createdType); err != nil {
		t.Fatalf("parse created account type: %v, body=%s", err, string(body))
	}
	if createdType.ID == "" {
		t.Fatalf("created account type has empty ID")
	}

	accountTypeID := createdType.ID
	t.Logf("Created account type: ID=%s Name=%s KeyValue=%s", accountTypeID, createdType.Name, createdType.KeyValue)

	// Verify created fields
	if createdType.Name != typeName {
		t.Errorf("name mismatch: got %q, want %q", createdType.Name, typeName)
	}
	if createdType.Description != description {
		t.Errorf("description mismatch: got %q, want %q", createdType.Description, description)
	}
	if createdType.OrganizationID != orgID {
		t.Errorf("organizationId mismatch: got %q, want %q", createdType.OrganizationID, orgID)
	}
	if createdType.LedgerID != ledgerID {
		t.Errorf("ledgerId mismatch: got %q, want %q", createdType.LedgerID, ledgerID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) LIST - GET all account types and verify created one is present
	// ─────────────────────────────────────────────────────────────────────────
	listResp, err := h.ListAccountTypes(ctx, onboard, headers, orgID, ledgerID)
	if err != nil {
		t.Fatalf("LIST account types failed: %v", err)
	}

	found := false
	for _, at := range listResp.Items {
		if at.ID == accountTypeID {
			found = true
			if at.Name != typeName {
				t.Errorf("listed type name mismatch: got %q, want %q", at.Name, typeName)
			}
			break
		}
	}
	if !found {
		t.Errorf("created account type not found in list: ID=%s", accountTypeID)
	}
	t.Logf("LIST returned %d account types, created type found=%v", len(listResp.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) UPDATE - PATCH description
	// ─────────────────────────────────────────────────────────────────────────
	updatedDescription := "Updated description for integration test"
	updatePayload := map[string]any{
		"description": updatedDescription,
		"metadata": map[string]any{
			"category":    "deposit",
			"environment": "test",
			"tier":        "premium",
			"updated":     true,
		},
	}

	updatePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types/%s", orgID, ledgerID, accountTypeID)
	code, body, err = onboard.Request(ctx, "PATCH", updatePath, headers, updatePayload)
	if err != nil {
		t.Fatalf("PATCH request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("UPDATE account type: expected %d, got %d, body=%s", http.StatusOK, code, string(body))
	}

	var updatedType h.AccountTypeResponse
	if err := json.Unmarshal(body, &updatedType); err != nil {
		t.Fatalf("parse updated account type: %v, body=%s", err, string(body))
	}

	t.Logf("Updated account type: ID=%s Description=%s", updatedType.ID, updatedType.Description)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) VERIFY - Fetch and confirm update was applied
	// ─────────────────────────────────────────────────────────────────────────
	verifyType, err := h.GetAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID)
	if err != nil {
		t.Fatalf("GET account type after update failed: %v", err)
	}

	if verifyType.Description != updatedDescription {
		t.Errorf("persisted description mismatch: got %q, want %q", verifyType.Description, updatedDescription)
	}
	// Name should remain unchanged
	if verifyType.Name != typeName {
		t.Errorf("name should not change: got %q, want %q", verifyType.Name, typeName)
	}
	t.Logf("Verified update applied: Description=%s", verifyType.Description)

	// ─────────────────────────────────────────────────────────────────────────
	// 5) DELETE - Remove account type
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteAccountType(ctx, onboard, headers, orgID, ledgerID, accountTypeID)
	if err != nil {
		t.Fatalf("DELETE account type failed: %v", err)
	}
	t.Logf("Deleted account type: ID=%s", accountTypeID)

	// ─────────────────────────────────────────────────────────────────────────
	// 6) VERIFY DELETE - GET should return 404 (with retry for replica lag tolerance)
	// ─────────────────────────────────────────────────────────────────────────
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types/%s", orgID, ledgerID, accountTypeID)
	h.WaitForDeletedWithRetry(t, "account type", func() error {
		code, _, err := onboard.Request(ctx, "GET", getPath, headers, nil)
		if err != nil {
			return err
		}
		if code == http.StatusNotFound {
			return fmt.Errorf("not found")
		}
		return nil
	})

	t.Log("AccountTypes CRUD lifecycle completed successfully")
}

// TestIntegration_AccountTypes_Validation tests validation and error handling for account types.
// This test verifies:
// 1. Missing required name field returns 400
// 2. Duplicate name/keyValue returns 409
// 3. GET non-existent type returns 404
// 4. PATCH non-existent type returns 404
// 5. DELETE non-existent type returns 404
func TestIntegration_AccountTypes_Validation(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization and Ledger
	// ─────────────────────────────────────────────────────────────────────────
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("AccountTypes Validation Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("AccountTypes Validation Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	basePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/account-types", orgID, ledgerID)
	nonExistentID := "00000000-0000-0000-0000-000000000000"

	// ─────────────────────────────────────────────────────────────────────────
	// 1) VALIDATION: Missing required name field should return 400
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("MissingRequiredName", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{
			"keyValue": fmt.Sprintf("no_name_%s", h.RandString(4)),
			// name is missing
		}
		code, body, err := onboard.Request(ctx, "POST", basePath, headers, payload)
		if err != nil {
			t.Logf("POST request error (may be expected): %v", err)
		}
		if code != http.StatusBadRequest {
			t.Errorf("missing name: expected %d, got %d, body=%s", http.StatusBadRequest, code, string(body))
		} else {
			t.Logf("Missing name correctly returned 400")
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// 2) VALIDATION: Missing required keyValue field should return 400
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("MissingRequiredKeyValue", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{
			"name": fmt.Sprintf("No KeyValue Type %s", h.RandString(4)),
			// keyValue is missing
		}
		code, body, err := onboard.Request(ctx, "POST", basePath, headers, payload)
		if err != nil {
			t.Logf("POST request error (may be expected): %v", err)
		}
		if code != http.StatusBadRequest {
			t.Errorf("missing keyValue: expected %d, got %d, body=%s", http.StatusBadRequest, code, string(body))
		} else {
			t.Logf("Missing keyValue correctly returned 400")
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// 3) VALIDATION: Duplicate keyValue should return 409
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("DuplicateKeyValue", func(t *testing.T) {
		t.Parallel()

		// Create unique headers for this subtest to avoid conflicts
		subHeaders := h.AuthHeaders(h.RandHex(8))

		sharedKeyValue := fmt.Sprintf("dup_key_%s", h.RandString(6))

		// Create first account type
		firstPayload := map[string]any{
			"name":     fmt.Sprintf("First Type %s", h.RandString(4)),
			"keyValue": sharedKeyValue,
		}
		code, body, err := onboard.Request(ctx, "POST", basePath, subHeaders, firstPayload)
		if err != nil {
			t.Fatalf("first POST request failed: %v", err)
		}
		if code != http.StatusCreated {
			t.Fatalf("first create: expected %d, got %d, body=%s", http.StatusCreated, code, string(body))
		}

		var firstType h.AccountTypeResponse
		if err := json.Unmarshal(body, &firstType); err != nil {
			t.Fatalf("parse first type: %v", err)
		}
		t.Logf("Created first account type: ID=%s KeyValue=%s", firstType.ID, firstType.KeyValue)

		// Cleanup first type
		t.Cleanup(func() {
			if delErr := h.DeleteAccountType(ctx, onboard, subHeaders, orgID, ledgerID, firstType.ID); delErr != nil {
				t.Logf("Warning: cleanup delete first type failed: %v", delErr)
			}
		})

		// Attempt to create second account type with same keyValue
		secondPayload := map[string]any{
			"name":     fmt.Sprintf("Second Type %s", h.RandString(4)),
			"keyValue": sharedKeyValue, // Same key!
		}
		code, body, err = onboard.Request(ctx, "POST", basePath, subHeaders, secondPayload)
		if err != nil {
			t.Logf("second POST request error (expected for duplicate): %v", err)
		}

		// Accept 409 Conflict or 400 Bad Request for duplicate
		if code == http.StatusCreated {
			t.Errorf("duplicate keyValue should be rejected, got %d: body=%s", code, string(body))
			// Cleanup accidentally created type
			var secondType h.AccountTypeResponse
			if json.Unmarshal(body, &secondType) == nil && secondType.ID != "" {
				if delErr := h.DeleteAccountType(ctx, onboard, subHeaders, orgID, ledgerID, secondType.ID); delErr != nil {
					t.Logf("Warning: cleanup delete accidental type failed: %v", delErr)
				}
			}
		} else if code == http.StatusConflict {
			t.Logf("Duplicate keyValue correctly returned 409 Conflict")
		} else if code == http.StatusBadRequest {
			t.Logf("Duplicate keyValue returned 400 Bad Request (acceptable)")
		} else {
			t.Errorf("duplicate keyValue: unexpected status %d, body=%s", code, string(body))
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// 4) NOT FOUND: GET non-existent type should return 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("GetNonExistent", func(t *testing.T) {
		t.Parallel()
		getPath := fmt.Sprintf("%s/%s", basePath, nonExistentID)
		code, body, err := onboard.Request(ctx, "GET", getPath, headers, nil)
		if err != nil {
			t.Logf("GET request error (may be expected): %v", err)
		}
		if code != http.StatusNotFound {
			t.Errorf("GET non-existent: expected %d, got %d, body=%s", http.StatusNotFound, code, string(body))
		} else {
			t.Logf("GET non-existent correctly returned 404")
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// 5) NOT FOUND: PATCH non-existent type should return 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("PatchNonExistent", func(t *testing.T) {
		t.Parallel()
		patchPath := fmt.Sprintf("%s/%s", basePath, nonExistentID)
		updatePayload := map[string]any{
			"description": "This should not work",
		}
		code, body, err := onboard.Request(ctx, "PATCH", patchPath, headers, updatePayload)
		if err != nil {
			t.Logf("PATCH request error (may be expected): %v", err)
		}
		if code != http.StatusNotFound {
			t.Errorf("PATCH non-existent: expected %d, got %d, body=%s", http.StatusNotFound, code, string(body))
		} else {
			t.Logf("PATCH non-existent correctly returned 404")
		}
	})

	// ─────────────────────────────────────────────────────────────────────────
	// 6) NOT FOUND: DELETE non-existent type should return 404
	// ─────────────────────────────────────────────────────────────────────────
	t.Run("DeleteNonExistent", func(t *testing.T) {
		t.Parallel()
		deletePath := fmt.Sprintf("%s/%s", basePath, nonExistentID)
		code, body, err := onboard.Request(ctx, "DELETE", deletePath, headers, nil)
		if err != nil {
			t.Logf("DELETE request error (may be expected): %v", err)
		}
		// Accept 404 or 204 (some APIs return success for delete of non-existent)
		if code != http.StatusNotFound && code != http.StatusNoContent {
			t.Errorf("DELETE non-existent: expected %d or %d, got %d, body=%s",
				http.StatusNotFound, http.StatusNoContent, code, string(body))
		} else {
			t.Logf("DELETE non-existent returned %d", code)
		}
	})

	t.Log("AccountTypes Validation tests completed")
}
