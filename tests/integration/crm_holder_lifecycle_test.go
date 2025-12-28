package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_CRM_HolderCRUDLifecycle tests the complete CRUD lifecycle for Holders.
// This covers CREATE, READ (single and list), UPDATE, and DELETE operations.
func TestIntegration_CRM_HolderCRUDLifecycle(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Test data
	holderName := fmt.Sprintf("Test Holder %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()
	updatedName := fmt.Sprintf("Updated Holder %s", h.RandString(6))

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - Natural Person Holder
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := h.CreateNaturalPersonPayload(holderName, holderCPF)
	createPayload["metadata"] = map[string]any{"environment": "test", "source": "integration"}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdHolder h.HolderResponse
	if err := json.Unmarshal(body, &createdHolder); err != nil || createdHolder.ID == "" {
		t.Fatalf("parse created holder: %v body=%s", err, string(body))
	}

	holderID := createdHolder.ID
	t.Logf("Created holder: ID=%s Name=%s Document=%s", holderID, createdHolder.Name, createdHolder.Document)

	// Verify created holder fields
	if createdHolder.Name != holderName {
		t.Errorf("holder name mismatch: got %q, want %q", createdHolder.Name, holderName)
	}
	if createdHolder.Document != holderCPF {
		t.Errorf("holder document mismatch: got %q, want %q", createdHolder.Document, holderCPF)
	}
	if createdHolder.Type != "NATURAL_PERSON" {
		t.Errorf("holder type mismatch: got %q, want %q", createdHolder.Type, "NATURAL_PERSON")
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - Get Holder by ID
	// ─────────────────────────────────────────────────────────────────────────
	fetchedHolder, err := h.GetHolder(ctx, crm, headers, holderID)
	if err != nil {
		t.Fatalf("GET holder by ID failed: %v", err)
	}

	if fetchedHolder.ID != holderID {
		t.Errorf("fetched holder ID mismatch: got %q, want %q", fetchedHolder.ID, holderID)
	}
	if fetchedHolder.Name != holderName {
		t.Errorf("fetched holder name mismatch: got %q, want %q", fetchedHolder.Name, holderName)
	}

	t.Logf("Fetched holder: ID=%s Name=%s", fetchedHolder.ID, fetchedHolder.Name)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) LIST - Get All Holders (verify our holder appears)
	// ─────────────────────────────────────────────────────────────────────────
	holderList, err := h.ListHolders(ctx, crm, headers)
	if err != nil {
		t.Fatalf("LIST holders failed: %v", err)
	}

	found := false
	for _, holder := range holderList.Items {
		if holder.ID == holderID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created holder not found in list: ID=%s", holderID)
	}

	t.Logf("List holders: found %d holders, target holder found=%v", len(holderList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) UPDATE - Modify Holder Name
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"environment": "test",
			"source":      "integration",
			"updated":     true,
		},
	}

	updatedHolder, err := h.UpdateHolder(ctx, crm, headers, holderID, updatePayload)
	if err != nil {
		t.Fatalf("UPDATE holder failed: %v", err)
	}

	if updatedHolder.Name != updatedName {
		t.Errorf("updated holder name mismatch: got %q, want %q", updatedHolder.Name, updatedName)
	}
	// Document should remain unchanged
	if updatedHolder.Document != holderCPF {
		t.Errorf("updated holder document should not change: got %q, want %q", updatedHolder.Document, holderCPF)
	}

	t.Logf("Updated holder: ID=%s NewName=%s", updatedHolder.ID, updatedHolder.Name)

	// Verify update persisted by fetching again
	verifyHolder, err := h.GetHolder(ctx, crm, headers, holderID)
	if err != nil {
		t.Fatalf("GET holder after update failed: %v", err)
	}
	if verifyHolder.Name != updatedName {
		t.Errorf("persisted name mismatch: got %q, want %q", verifyHolder.Name, updatedName)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 5) DELETE - Remove Holder
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteHolder(ctx, crm, headers, holderID)
	if err != nil {
		t.Fatalf("DELETE holder failed: %v", err)
	}

	t.Logf("Deleted holder: ID=%s", holderID)

	// Verify deletion - GET should fail
	_, err = h.GetHolder(ctx, crm, headers, holderID)
	if err == nil {
		t.Errorf("GET deleted holder should fail, but succeeded")
	}

	t.Log("Holder CRUD lifecycle completed successfully")
}

// TestIntegration_CRM_HolderLegalPerson tests creating a legal person (company) holder.
func TestIntegration_CRM_HolderLegalPerson(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Test data - Legal Person (company)
	companyName := fmt.Sprintf("Test Company %s LTDA", h.RandString(6))
	companyCNPJ := h.GenerateValidCNPJ()

	// CREATE - Legal Person Holder
	createPayload := h.CreateLegalPersonPayload(companyName, companyCNPJ)

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE legal person holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdHolder h.HolderResponse
	if err := json.Unmarshal(body, &createdHolder); err != nil || createdHolder.ID == "" {
		t.Fatalf("parse created holder: %v body=%s", err, string(body))
	}

	t.Logf("Created legal person holder: ID=%s Name=%s CNPJ=%s", createdHolder.ID, createdHolder.Name, createdHolder.Document)

	// Verify holder type
	if createdHolder.Type != "LEGAL_PERSON" {
		t.Errorf("holder type mismatch: got %q, want %q", createdHolder.Type, "LEGAL_PERSON")
	}

	// Cleanup
	if err := h.DeleteHolder(ctx, crm, headers, createdHolder.ID); err != nil {
		t.Logf("Warning: cleanup delete holder failed: %v", err)
	}

	t.Log("Legal person holder test completed successfully")
}

// TestIntegration_CRM_HolderValidation tests validation errors for holder creation.
func TestIntegration_CRM_HolderValidation(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Test missing required fields
	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "missing type",
			payload: map[string]any{"name": "Test", "document": "12345678901"},
		},
		{
			name:    "missing name",
			payload: map[string]any{"type": "NATURAL_PERSON", "document": "12345678901"},
		},
		{
			name:    "missing document",
			payload: map[string]any{"type": "NATURAL_PERSON", "name": "Test"},
		},
		{
			name:    "invalid type",
			payload: map[string]any{"type": "INVALID_TYPE", "name": "Test", "document": "12345678901"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, tc.payload)
			if err != nil {
				t.Logf("Request error (expected for validation): %v", err)
			}
			// Expect 400 Bad Request for validation errors
			if code == 201 {
				t.Errorf("expected validation error for %s, but got 201 Created: body=%s", tc.name, string(body))
			}
			t.Logf("Validation test %s: code=%d (expected non-201)", tc.name, code)
		})
	}
}

// TestIntegration_CRM_HolderDuplicateDocument tests that duplicate documents (CPF/CNPJ) are rejected.
// This is a critical business rule - each holder must have a unique document identifier.
func TestIntegration_CRM_HolderDuplicateDocument(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Generate a valid CPF for testing
	sharedCPF := h.GenerateValidCPF()

	// Create first holder with the CPF
	holder1Name := fmt.Sprintf("First Holder %s", h.RandString(6))
	holder1Payload := h.CreateNaturalPersonPayload(holder1Name, sharedCPF)

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, holder1Payload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE first holder failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var holder1 h.HolderResponse
	if err := json.Unmarshal(body, &holder1); err != nil {
		t.Fatalf("parse first holder: %v", err)
	}
	t.Logf("Created first holder: ID=%s CPF=%s", holder1.ID, sharedCPF)

	// Attempt to create second holder with SAME CPF - should fail
	holder2Name := fmt.Sprintf("Second Holder %s", h.RandString(6))
	holder2Payload := h.CreateNaturalPersonPayload(holder2Name, sharedCPF)

	code, body, err = crm.Request(ctx, "POST", "/v1/holders", headers, holder2Payload)
	if err != nil {
		t.Logf("Request error (may be expected for duplicate): %v", err)
	}

	// Should get 409 Conflict or 400 Bad Request for duplicate document
	if code == 201 {
		t.Errorf("duplicate CPF should be rejected, but got 201 Created: body=%s", string(body))
		// Cleanup the accidentally created holder
		var holder2 h.HolderResponse
		if json.Unmarshal(body, &holder2) == nil && holder2.ID != "" {
			_ = h.DeleteHolder(ctx, crm, headers, holder2.ID)
		}
	} else {
		t.Logf("Duplicate CPF correctly rejected: code=%d", code)
	}

	// Cleanup first holder
	if err := h.DeleteHolder(ctx, crm, headers, holder1.ID); err != nil {
		t.Logf("Warning: cleanup first holder failed: %v", err)
	}

	t.Log("Duplicate document validation test completed")
}
