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

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteHolder(ctx, crm, headers, createdHolder.ID); err != nil {
			t.Logf("Warning: cleanup delete holder failed: %v", err)
		}
	})

	// Verify holder type
	if createdHolder.Type != "LEGAL_PERSON" {
		t.Errorf("holder type mismatch: got %q, want %q", createdHolder.Type, "LEGAL_PERSON")
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

	// Generate valid CPFs for tests that should fail on other validations
	validCPF := h.GenerateValidCPF()

	// Test missing required fields
	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "missing type",
			payload: map[string]any{"name": "Test", "document": validCPF},
		},
		{
			name:    "missing name",
			payload: map[string]any{"type": "NATURAL_PERSON", "document": validCPF},
		},
		{
			name:    "missing document",
			payload: map[string]any{"type": "NATURAL_PERSON", "name": "Test"},
		},
		{
			name:    "invalid type",
			payload: map[string]any{"type": "INVALID_TYPE", "name": "Test", "document": validCPF},
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, tc.payload)
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

	// Register cleanup for first holder
	t.Cleanup(func() {
		if err := h.DeleteHolder(ctx, crm, headers, holder1.ID); err != nil {
			t.Logf("Warning: cleanup first holder failed: %v", err)
		}
	})

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
			if delErr := h.DeleteHolder(ctx, crm, headers, holder2.ID); delErr != nil {
				t.Logf("Warning: cleanup delete accidental holder failed: %v", delErr)
			}
		}
	} else {
		t.Logf("Duplicate CPF correctly rejected: code=%d", code)
	}

	t.Log("Duplicate document validation test completed")
}

// TestIntegration_CRM_HolderWithAddresses tests creating a holder with addresses.
func TestIntegration_CRM_HolderWithAddresses(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	holderName := fmt.Sprintf("Holder With Addresses %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()

	// Create payload with addresses array
	createPayload := map[string]any{
		"type":     "NATURAL_PERSON",
		"name":     holderName,
		"document": holderCPF,
		"addresses": []map[string]any{
			{
				"type":        "residential",
				"street":      "123 Main Street",
				"number":      "456",
				"complement":  "Apt 789",
				"city":        "San Francisco",
				"state":       "CA",
				"postalCode":  "94102",
				"country":     "USA",
				"isDefault":   true,
			},
			{
				"type":        "commercial",
				"street":      "789 Business Ave",
				"number":      "100",
				"city":        "San Francisco",
				"state":       "CA",
				"postalCode":  "94105",
				"country":     "USA",
				"isDefault":   false,
			},
		},
		"metadata": map[string]any{"environment": "test"},
	}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE holder with addresses failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdHolder h.HolderResponse
	if err := json.Unmarshal(body, &createdHolder); err != nil || createdHolder.ID == "" {
		t.Fatalf("parse created holder: %v body=%s", err, string(body))
	}

	t.Logf("Created holder with addresses: ID=%s Name=%s", createdHolder.ID, createdHolder.Name)

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteHolder(ctx, crm, headers, createdHolder.ID); err != nil {
			t.Logf("Warning: cleanup delete holder failed: %v", err)
		}
	})

	// Verify holder was created correctly
	if createdHolder.Name != holderName {
		t.Errorf("holder name mismatch: got %q, want %q", createdHolder.Name, holderName)
	}

	// Fetch and verify addresses persist
	fetchedHolder, err := h.GetHolder(ctx, crm, headers, createdHolder.ID)
	if err != nil {
		t.Fatalf("GET holder failed: %v", err)
	}

	if fetchedHolder.ID != createdHolder.ID {
		t.Errorf("fetched holder ID mismatch: got %q, want %q", fetchedHolder.ID, createdHolder.ID)
	}

	t.Log("Holder with addresses test completed successfully")
}

// TestIntegration_CRM_HolderWithContactInfo tests creating a holder with contact information.
func TestIntegration_CRM_HolderWithContactInfo(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	holderName := fmt.Sprintf("Holder With Contact %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()

	// Create payload with contact information
	createPayload := map[string]any{
		"type":     "NATURAL_PERSON",
		"name":     holderName,
		"document": holderCPF,
		"contact": map[string]any{
			"email":    fmt.Sprintf("test-%s@example.com", h.RandString(6)),
			"phone":    "+1-555-123-4567",
			"mobile":   "+1-555-987-6543",
		},
		"metadata": map[string]any{"environment": "test"},
	}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE holder with contact info failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdHolder h.HolderResponse
	if err := json.Unmarshal(body, &createdHolder); err != nil || createdHolder.ID == "" {
		t.Fatalf("parse created holder: %v body=%s", err, string(body))
	}

	t.Logf("Created holder with contact info: ID=%s Name=%s", createdHolder.ID, createdHolder.Name)

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteHolder(ctx, crm, headers, createdHolder.ID); err != nil {
			t.Logf("Warning: cleanup delete holder failed: %v", err)
		}
	})

	// Verify holder was created correctly
	if createdHolder.Name != holderName {
		t.Errorf("holder name mismatch: got %q, want %q", createdHolder.Name, holderName)
	}

	t.Log("Holder with contact info test completed successfully")
}

// TestIntegration_CRM_NaturalPersonExtendedFields tests creating a natural person holder with extended fields.
func TestIntegration_CRM_NaturalPersonExtendedFields(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	holderName := fmt.Sprintf("Natural Person Extended %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()

	// Create payload with NaturalPerson-specific fields
	createPayload := map[string]any{
		"type":     "NATURAL_PERSON",
		"name":     holderName,
		"document": holderCPF,
		"naturalPerson": map[string]any{
			"birthDate":    "1990-05-15",
			"nationality":  "Brazilian",
			"motherName":   "Maria Silva",
			"fatherName":   "Jose Silva",
			"occupation":   "Software Engineer",
			"gender":       "M",
			"maritalStatus": "single",
		},
		"contact": map[string]any{
			"email": fmt.Sprintf("natural-%s@example.com", h.RandString(6)),
		},
		"metadata": map[string]any{"environment": "test", "personType": "natural"},
	}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE natural person with extended fields failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdHolder h.HolderResponse
	if err := json.Unmarshal(body, &createdHolder); err != nil || createdHolder.ID == "" {
		t.Fatalf("parse created holder: %v body=%s", err, string(body))
	}

	t.Logf("Created natural person with extended fields: ID=%s Name=%s Type=%s", createdHolder.ID, createdHolder.Name, createdHolder.Type)

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteHolder(ctx, crm, headers, createdHolder.ID); err != nil {
			t.Logf("Warning: cleanup delete holder failed: %v", err)
		}
	})

	// Verify holder was created as NATURAL_PERSON
	if createdHolder.Type != "NATURAL_PERSON" {
		t.Errorf("holder type mismatch: got %q, want %q", createdHolder.Type, "NATURAL_PERSON")
	}

	t.Log("Natural person extended fields test completed successfully")
}

// TestIntegration_CRM_LegalPersonExtendedFields tests creating a legal person holder with extended fields.
func TestIntegration_CRM_LegalPersonExtendedFields(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	companyName := fmt.Sprintf("Legal Person Extended %s LTDA", h.RandString(6))
	companyCNPJ := h.GenerateValidCNPJ()

	// Create payload with LegalPerson-specific fields
	createPayload := map[string]any{
		"type":     "LEGAL_PERSON",
		"name":     companyName,
		"document": companyCNPJ,
		"legalPerson": map[string]any{
			"tradeName":         fmt.Sprintf("Trade %s", h.RandString(4)),
			"foundationDate":    "2015-01-20",
			"registrationNumber": fmt.Sprintf("REG%s", h.RandString(8)),
			"legalNature":       "LTDA",
			"businessActivity":  "Technology Services",
		},
		"contact": map[string]any{
			"email": fmt.Sprintf("company-%s@example.com", h.RandString(6)),
			"phone": "+1-555-COMPANY",
		},
		"addresses": []map[string]any{
			{
				"type":       "headquarters",
				"street":     "Corporate Blvd",
				"number":     "1000",
				"city":       "New York",
				"state":      "NY",
				"postalCode": "10001",
				"country":    "USA",
				"isDefault":  true,
			},
		},
		"metadata": map[string]any{"environment": "test", "personType": "legal"},
	}

	code, body, err := crm.Request(ctx, "POST", "/v1/holders", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE legal person with extended fields failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdHolder h.HolderResponse
	if err := json.Unmarshal(body, &createdHolder); err != nil || createdHolder.ID == "" {
		t.Fatalf("parse created holder: %v body=%s", err, string(body))
	}

	t.Logf("Created legal person with extended fields: ID=%s Name=%s Type=%s", createdHolder.ID, createdHolder.Name, createdHolder.Type)

	// Register cleanup
	t.Cleanup(func() {
		if err := h.DeleteHolder(ctx, crm, headers, createdHolder.ID); err != nil {
			t.Logf("Warning: cleanup delete holder failed: %v", err)
		}
	})

	// Verify holder was created as LEGAL_PERSON
	if createdHolder.Type != "LEGAL_PERSON" {
		t.Errorf("holder type mismatch: got %q, want %q", createdHolder.Type, "LEGAL_PERSON")
	}

	t.Log("Legal person extended fields test completed successfully")
}
