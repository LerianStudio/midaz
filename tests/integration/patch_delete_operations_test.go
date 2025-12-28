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
