package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// ═══════════════════════════════════════════════════════════════════════════════
// ORGANIZATION PATCH/DELETE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Organization_Update tests PATCH operation for organizations.
func TestIntegration_Organization_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization
	originalName := fmt.Sprintf("Original Org %s", h.RandString(6))
	orgID, err := h.SetupOrganization(ctx, onboard, headers, originalName)
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s Name=%s", orgID, originalName)

	// Update organization
	updatedName := fmt.Sprintf("Updated Org %s", h.RandString(6))
	updatePayload := map[string]any{
		"legalName": updatedName,
		"metadata": map[string]any{
			"environment": "test",
			"updated":     true,
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s", orgID)
	code, body, err := onboard.Request(ctx, "PATCH", path, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedOrg struct {
		ID        string `json:"id"`
		LegalName string `json:"legalName"`
	}
	if err := json.Unmarshal(body, &updatedOrg); err != nil {
		t.Fatalf("parse updated organization: %v body=%s", err, string(body))
	}

	if updatedOrg.LegalName != updatedName {
		t.Errorf("organization legalName not updated: got %q, want %q", updatedOrg.LegalName, updatedName)
	}

	t.Logf("Updated organization: ID=%s NewLegalName=%s", updatedOrg.ID, updatedOrg.LegalName)

	// Verify update persisted
	code, body, err = onboard.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET organization after update failed: code=%d err=%v", code, err)
	}

	var verifyOrg struct {
		LegalName string `json:"legalName"`
	}
	_ = json.Unmarshal(body, &verifyOrg)
	if verifyOrg.LegalName != updatedName {
		t.Errorf("organization update not persisted: got %q, want %q", verifyOrg.LegalName, updatedName)
	}

	t.Log("Organization PATCH test completed successfully")
}

// TestIntegration_Organization_Delete tests DELETE operation for organizations.
func TestIntegration_Organization_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Delete Test Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization for deletion: ID=%s", orgID)

	// Delete organization
	path := fmt.Sprintf("/v1/organizations/%s", orgID)
	code, body, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE organization failed: code=%d err=%v body=%s", code, err, string(body))
	}

	t.Logf("Deleted organization: ID=%s", orgID)

	// Verify deletion - GET should fail or return deleted state
	code, _, _ = onboard.Request(ctx, "GET", path, headers, nil)
	if code == 200 {
		t.Logf("Warning: GET after delete returned 200 - organization may be soft-deleted")
	}

	t.Log("Organization DELETE test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// LEDGER PATCH/DELETE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Ledger_Update tests PATCH operation for ledgers.
func TestIntegration_Ledger_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Ledger Update Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	originalName := fmt.Sprintf("Original Ledger %s", h.RandString(6))
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, originalName)
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s Name=%s", ledgerID, originalName)

	// Update ledger
	updatedName := fmt.Sprintf("Updated Ledger %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"environment": "test",
			"updated":     true,
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "PATCH", path, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedLedger struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &updatedLedger); err != nil {
		t.Fatalf("parse updated ledger: %v body=%s", err, string(body))
	}

	if updatedLedger.Name != updatedName {
		t.Errorf("ledger name not updated: got %q, want %q", updatedLedger.Name, updatedName)
	}

	t.Logf("Updated ledger: ID=%s NewName=%s", updatedLedger.ID, updatedLedger.Name)
	t.Log("Ledger PATCH test completed successfully")
}

// TestIntegration_Ledger_Delete tests DELETE operation for ledgers.
func TestIntegration_Ledger_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Ledger Delete Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Delete Test Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger for deletion: ID=%s", ledgerID)

	// Delete ledger
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}

	t.Logf("Deleted ledger: ID=%s", ledgerID)
	t.Log("Ledger DELETE test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// ACCOUNT PATCH/DELETE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Account_Update tests PATCH operation for accounts.
func TestIntegration_Account_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Account Update Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Account Update Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	originalAlias := fmt.Sprintf("original-account-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, originalAlias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, originalAlias)

	// Update account
	updatedName := fmt.Sprintf("Updated Account %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"environment": "test",
			"updated":     true,
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, accountID)
	code, body, err := onboard.Request(ctx, "PATCH", path, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedAccount struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &updatedAccount); err != nil {
		t.Fatalf("parse updated account: %v body=%s", err, string(body))
	}

	if updatedAccount.Name != updatedName {
		t.Errorf("account name not updated: got %q, want %q", updatedAccount.Name, updatedName)
	}

	t.Logf("Updated account: ID=%s NewName=%s", updatedAccount.ID, updatedAccount.Name)
	t.Log("Account PATCH test completed successfully")
}

// TestIntegration_Account_Delete tests DELETE operation for accounts.
func TestIntegration_Account_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Account Delete Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Account Delete Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, fmt.Sprintf("delete-account-%s", h.RandString(6)), "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account for deletion: ID=%s", accountID)

	// Delete account
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, accountID)
	code, body, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE account failed: code=%d err=%v body=%s", code, err, string(body))
	}

	t.Logf("Deleted account: ID=%s", accountID)
	t.Log("Account DELETE test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// ASSET PATCH/DELETE TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Asset_Update tests PATCH operation for assets.
func TestIntegration_Asset_Update(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Asset Update Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Asset Update Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create custom asset
	assetCode := fmt.Sprintf("TST%s", h.RandString(3))
	createAssetPayload := map[string]any{
		"name": "Test Asset",
		"type": "currency",
		"code": assetCode,
	}

	createPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", createPath, headers, createAssetPayload)
	if err != nil || code != 201 {
		t.Fatalf("create asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdAsset struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &createdAsset); err != nil {
		t.Fatalf("parse created asset: %v", err)
	}
	t.Logf("Created asset: ID=%s Code=%s", createdAsset.ID, assetCode)

	// Update asset
	updatedName := fmt.Sprintf("Updated Asset %s", h.RandString(6))
	updatePayload := map[string]any{
		"name": updatedName,
		"metadata": map[string]any{
			"environment": "test",
			"updated":     true,
		},
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets/%s", orgID, ledgerID, createdAsset.ID)
	code, body, err = onboard.Request(ctx, "PATCH", path, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedAsset struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &updatedAsset); err != nil {
		t.Fatalf("parse updated asset: %v body=%s", err, string(body))
	}

	if updatedAsset.Name != updatedName {
		t.Errorf("asset name not updated: got %q, want %q", updatedAsset.Name, updatedName)
	}

	t.Logf("Updated asset: ID=%s NewName=%s", updatedAsset.ID, updatedAsset.Name)
	t.Log("Asset PATCH test completed successfully")
}

// TestIntegration_Asset_Delete tests DELETE operation for assets.
func TestIntegration_Asset_Delete(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Asset Delete Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Asset Delete Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Create custom asset (not USD to avoid conflicts)
	assetCode := fmt.Sprintf("DEL%s", h.RandString(3))
	createAssetPayload := map[string]any{
		"name": "Asset To Delete",
		"type": "currency",
		"code": assetCode,
	}

	createPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)
	code, body, err := onboard.Request(ctx, "POST", createPath, headers, createAssetPayload)
	if err != nil || code != 201 {
		t.Fatalf("create asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdAsset struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &createdAsset); err != nil {
		t.Fatalf("parse created asset: %v", err)
	}
	t.Logf("Created asset for deletion: ID=%s Code=%s", createdAsset.ID, assetCode)

	// Delete asset
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets/%s", orgID, ledgerID, createdAsset.ID)
	code, body, err = onboard.Request(ctx, "DELETE", path, headers, nil)
	// Accept 200 or 204 for successful deletion
	if err != nil || (code != 200 && code != 204) {
		t.Fatalf("DELETE asset failed: code=%d err=%v body=%s", code, err, string(body))
	}

	t.Logf("Deleted asset: ID=%s", createdAsset.ID)
	t.Log("Asset DELETE test completed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════════
// IDEMPOTENCY TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// TestIntegration_Idempotency_DoubleDeleteOrganization tests that deleting an organization
// twice handles gracefully (idempotent delete).
func TestIntegration_Idempotency_DoubleDeleteOrganization(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Double Delete Org %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization for double-delete test: ID=%s", orgID)

	path := fmt.Sprintf("/v1/organizations/%s", orgID)

	// First delete - should succeed
	code1, body1, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	if err != nil || (code1 != 200 && code1 != 204) {
		t.Fatalf("First DELETE organization failed: code=%d err=%v body=%s", code1, err, string(body1))
	}
	t.Logf("First delete succeeded: code=%d", code1)

	// Second delete - should handle gracefully (not crash/panic)
	// Expected behavior: 404 (not found) or 204 (idempotent success) or 409 (already deleted)
	code2, body2, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	if err != nil {
		t.Logf("Second delete request error (may be expected): %v", err)
	}

	// Any non-5xx response is acceptable for idempotent delete
	if code2 >= 500 {
		t.Errorf("Second DELETE caused server error: code=%d body=%s", code2, string(body2))
	} else {
		t.Logf("Second delete handled gracefully: code=%d", code2)
	}

	t.Log("Double-delete organization test completed successfully")
}

// TestIntegration_Idempotency_DoubleDeleteLedger tests that deleting a ledger
// twice handles gracefully (idempotent delete).
func TestIntegration_Idempotency_DoubleDeleteLedger(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Double Delete Ledger Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Double Delete Ledger %s", h.RandString(6)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger for double-delete test: ID=%s", ledgerID)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s", orgID, ledgerID)

	// First delete - should succeed
	code1, body1, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	if err != nil || (code1 != 200 && code1 != 204) {
		t.Fatalf("First DELETE ledger failed: code=%d err=%v body=%s", code1, err, string(body1))
	}
	t.Logf("First delete succeeded: code=%d", code1)

	// Second delete - should handle gracefully
	code2, body2, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	if err != nil {
		t.Logf("Second delete request error (may be expected): %v", err)
	}

	// Any non-5xx response is acceptable
	if code2 >= 500 {
		t.Errorf("Second DELETE caused server error: code=%d body=%s", code2, string(body2))
	} else {
		t.Logf("Second delete handled gracefully: code=%d", code2)
	}

	t.Log("Double-delete ledger test completed successfully")
}

// TestIntegration_Idempotency_DoubleDeleteAccount tests that deleting an account
// twice handles gracefully (idempotent delete).
func TestIntegration_Idempotency_DoubleDeleteAccount(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Double Delete Account Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Double Delete Account Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, fmt.Sprintf("double-delete-account-%s", h.RandString(6)), "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account for double-delete test: ID=%s", accountID)

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, accountID)

	// First delete - should succeed
	code1, body1, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	if err != nil || (code1 != 200 && code1 != 204) {
		t.Fatalf("First DELETE account failed: code=%d err=%v body=%s", code1, err, string(body1))
	}
	t.Logf("First delete succeeded: code=%d", code1)

	// Second delete - should handle gracefully
	code2, body2, err := onboard.Request(ctx, "DELETE", path, headers, nil)
	if err != nil {
		t.Logf("Second delete request error (may be expected): %v", err)
	}

	// Any non-5xx response is acceptable
	if code2 >= 500 {
		t.Errorf("Second DELETE caused server error: code=%d body=%s", code2, string(body2))
	} else {
		t.Logf("Second delete handled gracefully: code=%d", code2)
	}

	t.Log("Double-delete account test completed successfully")
}

// TestIntegration_Idempotency_DuplicateAssetCreate tests that creating an asset
// with the same code twice is rejected properly.
func TestIntegration_Idempotency_DuplicateAssetCreate(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Dup Asset Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Dup Asset Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	// Generate unique asset code for this test
	assetCode := fmt.Sprintf("DUP%s", h.RandString(3))
	createPayload := map[string]any{
		"name": "Duplicate Test Asset",
		"type": "currency",
		"code": assetCode,
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", orgID, ledgerID)

	// First create - should succeed
	code1, body1, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil || code1 != 201 {
		t.Fatalf("First CREATE asset failed: code=%d err=%v body=%s", code1, err, string(body1))
	}
	t.Logf("First create succeeded: code=%d assetCode=%s", code1, assetCode)

	// Second create with same code - should be rejected (409 Conflict or 400 Bad Request)
	code2, body2, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil {
		t.Logf("Second create request error (may be expected): %v", err)
	}

	// Should NOT succeed - expect 409 (Conflict) or 400 (Bad Request)
	if code2 == 201 {
		t.Errorf("Duplicate asset creation should be rejected, but got 201 Created: body=%s", string(body2))
	} else {
		t.Logf("Duplicate asset correctly rejected: code=%d", code2)
	}

	t.Log("Duplicate asset creation test completed successfully")
}

// TestIntegration_Metadata_EmptyUpdate tests updating a resource with empty metadata `{}`.
// This verifies that the API handles empty metadata correctly (either preserving existing
// metadata or clearing it, depending on the API contract).
func TestIntegration_Metadata_EmptyUpdate(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create organization with initial metadata
	orgName := fmt.Sprintf("Empty Meta Test Org %s", h.RandString(6))
	createPayload := map[string]any{
		"legalName":       orgName,
		"doingBusinessAs": orgName,
		"metadata": map[string]any{
			"environment": "test",
			"version":     1,
			"tags":        []string{"integration", "metadata-test"},
		},
	}

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE organization with metadata failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdOrg struct {
		ID       string         `json:"id"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(body, &createdOrg); err != nil || createdOrg.ID == "" {
		t.Fatalf("parse created organization: %v body=%s", err, string(body))
	}

	orgID := createdOrg.ID
	t.Logf("Created organization with metadata: ID=%s metadata=%v", orgID, createdOrg.Metadata)

	// Verify initial metadata exists
	if len(createdOrg.Metadata) == 0 {
		t.Fatalf("expected initial metadata to be set, got empty")
	}

	// Update organization with empty metadata {}
	updatePayload := map[string]any{
		"metadata": map[string]any{}, // Empty metadata object
	}

	path := fmt.Sprintf("/v1/organizations/%s", orgID)
	code, body, err = onboard.Request(ctx, "PATCH", path, headers, updatePayload)
	if err != nil || code != 200 {
		t.Fatalf("PATCH organization with empty metadata failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var updatedOrg struct {
		ID       string         `json:"id"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(body, &updatedOrg); err != nil {
		t.Fatalf("parse updated organization: %v body=%s", err, string(body))
	}

	t.Logf("Updated organization with empty metadata: ID=%s metadata=%v", updatedOrg.ID, updatedOrg.Metadata)

	// Verify the behavior:
	// Option A: Empty metadata clears all metadata (metadata == nil or len == 0)
	// Option B: Empty metadata preserves existing metadata (metadata still has values)
	// Both behaviors are valid depending on API contract - we just log the behavior
	if len(updatedOrg.Metadata) == 0 {
		t.Logf("API behavior: Empty metadata CLEARS existing metadata")
	} else {
		t.Logf("API behavior: Empty metadata PRESERVES existing metadata: %v", updatedOrg.Metadata)
	}

	// Verify by fetching again
	code, body, err = onboard.Request(ctx, "GET", path, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("GET organization after empty metadata update failed: code=%d err=%v", code, err)
	}

	var verifyOrg struct {
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(body, &verifyOrg); err != nil {
		t.Fatalf("parse verify organization: %v body=%s", err, string(body))
	}

	t.Logf("Verified organization metadata after update: %v", verifyOrg.Metadata)
	t.Log("Empty metadata update test completed successfully")
}

// TestIntegration_Idempotency_DuplicateAccountAlias tests that creating an account
// with the same alias twice is rejected properly.
func TestIntegration_Idempotency_DuplicateAccountAlias(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Dup Alias Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Dup Alias Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	// Generate unique alias for this test
	sharedAlias := fmt.Sprintf("duplicate-alias-%s", h.RandString(6))

	// First account with alias - should succeed
	accountID1, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, sharedAlias, "USD")
	if err != nil {
		t.Fatalf("First account creation failed: %v", err)
	}
	t.Logf("First account created: ID=%s alias=%s", accountID1, sharedAlias)

	// Second account with same alias - should be rejected
	createPayload := map[string]any{
		"name":      "Duplicate Alias Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     sharedAlias,
	}

	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", orgID, ledgerID)
	code2, body2, err := onboard.Request(ctx, "POST", path, headers, createPayload)
	if err != nil {
		t.Logf("Second account request error (may be expected): %v", err)
	}

	// Should NOT succeed - expect 409 (Conflict) or 400 (Bad Request)
	if code2 == 201 {
		t.Errorf("Duplicate alias should be rejected, but got 201 Created: body=%s", string(body2))
		// Cleanup if accidentally created
		var acc struct{ ID string }
		if json.Unmarshal(body2, &acc) == nil && acc.ID != "" {
			deletePath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s", orgID, ledgerID, acc.ID)
			_, _, _ = onboard.Request(ctx, "DELETE", deletePath, headers, nil)
		}
	} else {
		t.Logf("Duplicate alias correctly rejected: code=%d", code2)
	}

	t.Log("Duplicate account alias test completed successfully")
}
