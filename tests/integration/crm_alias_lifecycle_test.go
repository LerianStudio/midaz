package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_CRM_AliasCRUDLifecycle tests the complete CRUD lifecycle for Aliases.
// This requires setting up: Organization → Ledger → Account → Holder → Alias
func TestIntegration_CRM_AliasCRUDLifecycle(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	baseHeaders := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Asset → Account
	// ─────────────────────────────────────────────────────────────────────────

	// Create Organization
	orgID, err := h.SetupOrganization(ctx, onboard, baseHeaders, fmt.Sprintf("Alias Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// CRM headers need organization ID
	crmHeaders := h.AuthHeadersWithOrg(h.RandHex(8), orgID)

	// Create Ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, baseHeaders, orgID, fmt.Sprintf("Alias Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create USD Asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, baseHeaders); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Create Account
	accountAlias := fmt.Sprintf("alias-test-account-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, baseHeaders, orgID, ledgerID, accountAlias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, accountAlias)

	// Create Holder (using CRM headers with org ID)
	holderName := fmt.Sprintf("Alias Test Holder %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()
	holderID, err := h.SetupHolder(ctx, crm, crmHeaders, holderName, holderCPF, "NATURAL_PERSON")
	if err != nil {
		t.Fatalf("setup holder failed: %v", err)
	}
	t.Logf("Created holder: ID=%s Name=%s", holderID, holderName)

	// ─────────────────────────────────────────────────────────────────────────
	// 1) CREATE - Alias linking Holder to Account
	// ─────────────────────────────────────────────────────────────────────────
	createPayload := h.CreateAliasPayload(ledgerID, accountID)
	createPayload["metadata"] = map[string]any{"environment": "test", "source": "integration"}

	path := fmt.Sprintf("/v1/holders/%s/aliases", holderID)
	code, body, err := crm.Request(ctx, "POST", path, crmHeaders, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE alias failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdAlias h.AliasResponse
	if err := json.Unmarshal(body, &createdAlias); err != nil || createdAlias.ID == "" {
		t.Fatalf("parse created alias: %v body=%s", err, string(body))
	}

	aliasID := createdAlias.ID
	t.Logf("Created alias: ID=%s HolderID=%s LedgerID=%s AccountID=%s",
		aliasID, createdAlias.HolderID, createdAlias.LedgerID, createdAlias.AccountID)

	// Verify created alias fields
	if createdAlias.HolderID != holderID {
		t.Errorf("alias holder ID mismatch: got %q, want %q", createdAlias.HolderID, holderID)
	}
	if createdAlias.LedgerID != ledgerID {
		t.Errorf("alias ledger ID mismatch: got %q, want %q", createdAlias.LedgerID, ledgerID)
	}
	if createdAlias.AccountID != accountID {
		t.Errorf("alias account ID mismatch: got %q, want %q", createdAlias.AccountID, accountID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 2) READ - Get Alias by ID
	// ─────────────────────────────────────────────────────────────────────────
	fetchedAlias, err := h.GetAlias(ctx, crm, crmHeaders, holderID, aliasID)
	if err != nil {
		t.Fatalf("GET alias by ID failed: %v", err)
	}

	if fetchedAlias.ID != aliasID {
		t.Errorf("fetched alias ID mismatch: got %q, want %q", fetchedAlias.ID, aliasID)
	}
	if fetchedAlias.AccountID != accountID {
		t.Errorf("fetched alias account ID mismatch: got %q, want %q", fetchedAlias.AccountID, accountID)
	}

	t.Logf("Fetched alias: ID=%s AccountID=%s", fetchedAlias.ID, fetchedAlias.AccountID)

	// ─────────────────────────────────────────────────────────────────────────
	// 3) LIST - Get All Aliases for Holder (verify our alias appears)
	// ─────────────────────────────────────────────────────────────────────────
	aliasList, err := h.ListAliases(ctx, crm, crmHeaders, holderID)
	if err != nil {
		t.Fatalf("LIST aliases failed: %v", err)
	}

	found := false
	for _, alias := range aliasList.Items {
		if alias.ID == aliasID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created alias not found in list: ID=%s", aliasID)
	}

	t.Logf("List aliases: found %d aliases, target alias found=%v", len(aliasList.Items), found)

	// ─────────────────────────────────────────────────────────────────────────
	// 4) LIST ALL - Get All Aliases across all Holders
	// ─────────────────────────────────────────────────────────────────────────
	allAliases, err := h.ListAllAliases(ctx, crm, crmHeaders)
	if err != nil {
		t.Fatalf("LIST all aliases failed: %v", err)
	}

	foundInAll := false
	for _, alias := range allAliases.Items {
		if alias.ID == aliasID {
			foundInAll = true
			break
		}
	}
	if !foundInAll {
		t.Errorf("created alias not found in global list: ID=%s", aliasID)
	}

	t.Logf("List all aliases: found %d aliases globally, target alias found=%v", len(allAliases.Items), foundInAll)

	// ─────────────────────────────────────────────────────────────────────────
	// 5) UPDATE - Modify Alias Metadata
	// ─────────────────────────────────────────────────────────────────────────
	updatePayload := map[string]any{
		"metadata": map[string]any{
			"environment": "test",
			"source":      "integration",
			"updated":     true,
			"version":     2,
		},
	}

	updatedAlias, err := h.UpdateAlias(ctx, crm, crmHeaders, holderID, aliasID, updatePayload)
	if err != nil {
		t.Fatalf("UPDATE alias failed: %v", err)
	}

	// Core fields should remain unchanged
	if updatedAlias.AccountID != accountID {
		t.Errorf("updated alias account ID should not change: got %q, want %q", updatedAlias.AccountID, accountID)
	}
	if updatedAlias.LedgerID != ledgerID {
		t.Errorf("updated alias ledger ID should not change: got %q, want %q", updatedAlias.LedgerID, ledgerID)
	}

	t.Logf("Updated alias: ID=%s", updatedAlias.ID)

	// Verify update persisted by fetching again
	verifyAlias, err := h.GetAlias(ctx, crm, crmHeaders, holderID, aliasID)
	if err != nil {
		t.Fatalf("GET alias after update failed: %v", err)
	}
	if verifyAlias.ID != aliasID {
		t.Errorf("verified alias ID mismatch: got %q, want %q", verifyAlias.ID, aliasID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 6) DELETE - Remove Alias
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteAlias(ctx, crm, crmHeaders, holderID, aliasID)
	if err != nil {
		t.Fatalf("DELETE alias failed: %v", err)
	}

	t.Logf("Deleted alias: ID=%s", aliasID)

	// Verify deletion - GET should fail
	_, err = h.GetAlias(ctx, crm, crmHeaders, holderID, aliasID)
	if err == nil {
		t.Errorf("GET deleted alias should fail, but succeeded")
	}

	// ─────────────────────────────────────────────────────────────────────────
	// CLEANUP
	// ─────────────────────────────────────────────────────────────────────────
	if err := h.DeleteHolder(ctx, crm, crmHeaders, holderID); err != nil {
		t.Logf("Warning: cleanup delete holder failed: %v", err)
	}

	t.Log("Alias CRUD lifecycle completed successfully")
}

// TestIntegration_CRM_MultipleAliasesPerHolder tests creating multiple aliases for a single holder.
func TestIntegration_CRM_MultipleAliasesPerHolder(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	baseHeaders := h.AuthHeaders(h.RandHex(8))

	// Setup: Organization → Ledger → Asset → 2 Accounts
	orgID, err := h.SetupOrganization(ctx, onboard, baseHeaders, fmt.Sprintf("Multi Alias Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	// CRM headers need organization ID
	crmHeaders := h.AuthHeadersWithOrg(h.RandHex(8), orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, baseHeaders, orgID, fmt.Sprintf("Multi Alias Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, baseHeaders); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	// Create two accounts
	account1ID, err := h.SetupAccount(ctx, onboard, baseHeaders, orgID, ledgerID, fmt.Sprintf("account1-%s", h.RandString(6)), "USD")
	if err != nil {
		t.Fatalf("setup account 1 failed: %v", err)
	}

	account2ID, err := h.SetupAccount(ctx, onboard, baseHeaders, orgID, ledgerID, fmt.Sprintf("account2-%s", h.RandString(6)), "USD")
	if err != nil {
		t.Fatalf("setup account 2 failed: %v", err)
	}

	// Create Holder
	holderID, err := h.SetupHolder(ctx, crm, crmHeaders, fmt.Sprintf("Multi Alias Holder %s", h.RandString(6)), h.GenerateValidCPF(), "NATURAL_PERSON")
	if err != nil {
		t.Fatalf("setup holder failed: %v", err)
	}

	// Create first alias
	alias1ID, err := h.SetupAlias(ctx, crm, crmHeaders, holderID, ledgerID, account1ID)
	if err != nil {
		t.Fatalf("create alias 1 failed: %v", err)
	}
	t.Logf("Created alias 1: ID=%s for account %s", alias1ID, account1ID)

	// Create second alias
	alias2ID, err := h.SetupAlias(ctx, crm, crmHeaders, holderID, ledgerID, account2ID)
	if err != nil {
		t.Fatalf("create alias 2 failed: %v", err)
	}
	t.Logf("Created alias 2: ID=%s for account %s", alias2ID, account2ID)

	// List aliases for holder - should have both
	aliasList, err := h.ListAliases(ctx, crm, crmHeaders, holderID)
	if err != nil {
		t.Fatalf("list aliases failed: %v", err)
	}

	if len(aliasList.Items) < 2 {
		t.Errorf("expected at least 2 aliases, got %d", len(aliasList.Items))
	}

	// Verify both aliases are present
	foundAlias1, foundAlias2 := false, false
	for _, alias := range aliasList.Items {
		if alias.ID == alias1ID {
			foundAlias1 = true
		}
		if alias.ID == alias2ID {
			foundAlias2 = true
		}
	}

	if !foundAlias1 {
		t.Errorf("alias 1 not found in holder's aliases")
	}
	if !foundAlias2 {
		t.Errorf("alias 2 not found in holder's aliases")
	}

	t.Logf("Multiple aliases test passed: holder has %d aliases", len(aliasList.Items))

	// Cleanup
	if err := h.DeleteAlias(ctx, crm, crmHeaders, holderID, alias1ID); err != nil {
		t.Logf("Warning: cleanup delete alias1 failed: %v", err)
	}
	if err := h.DeleteAlias(ctx, crm, crmHeaders, holderID, alias2ID); err != nil {
		t.Logf("Warning: cleanup delete alias2 failed: %v", err)
	}
	if err := h.DeleteHolder(ctx, crm, crmHeaders, holderID); err != nil {
		t.Logf("Warning: cleanup delete holder failed: %v", err)
	}
}

// TestIntegration_CRM_AliasWithBankingDetails tests creating an alias with banking details.
func TestIntegration_CRM_AliasWithBankingDetails(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	crm := h.NewHTTPClient(env.CRMURL, env.HTTPTimeout)
	baseHeaders := h.AuthHeaders(h.RandHex(8))

	// Setup: Organization -> Ledger -> Asset -> Account
	orgID, err := h.SetupOrganization(ctx, onboard, baseHeaders, fmt.Sprintf("Banking Details Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// CRM headers need organization ID
	crmHeaders := h.AuthHeadersWithOrg(h.RandHex(8), orgID)

	ledgerID, err := h.SetupLedger(ctx, onboard, baseHeaders, orgID, fmt.Sprintf("Banking Details Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, baseHeaders); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	accountAlias := fmt.Sprintf("banking-account-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, baseHeaders, orgID, ledgerID, accountAlias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, accountAlias)

	// Create Holder
	holderName := fmt.Sprintf("Banking Details Holder %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()
	holderID, err := h.SetupHolder(ctx, crm, crmHeaders, holderName, holderCPF, "NATURAL_PERSON")
	if err != nil {
		t.Fatalf("setup holder failed: %v", err)
	}
	t.Logf("Created holder: ID=%s Name=%s", holderID, holderName)

	// Create alias with banking details
	createPayload := map[string]any{
		"ledgerId":  ledgerID,
		"accountId": accountID,
		"bankingDetails": map[string]any{
			"bankId":      "001",
			"branch":      "1234",
			"account":     "123456789",
			"type":        "CACC",
			"openingDate": "2025-01-01",
			"countryCode": "US",
		},
		"metadata": map[string]any{
			"environment": "test",
			"hasBanking":  true,
		},
	}

	path := fmt.Sprintf("/v1/holders/%s/aliases", holderID)
	code, body, err := crm.Request(ctx, "POST", path, crmHeaders, createPayload)
	if err != nil || code != 201 {
		t.Fatalf("CREATE alias with banking details failed: code=%d err=%v body=%s", code, err, string(body))
	}

	var createdAlias h.AliasResponse
	if err := json.Unmarshal(body, &createdAlias); err != nil || createdAlias.ID == "" {
		t.Fatalf("parse created alias: %v body=%s", err, string(body))
	}

	t.Logf("Created alias with banking details: ID=%s HolderID=%s AccountID=%s",
		createdAlias.ID, createdAlias.HolderID, createdAlias.AccountID)

	// Verify alias was created correctly
	if createdAlias.HolderID != holderID {
		t.Errorf("alias holder ID mismatch: got %q, want %q", createdAlias.HolderID, holderID)
	}
	if createdAlias.AccountID != accountID {
		t.Errorf("alias account ID mismatch: got %q, want %q", createdAlias.AccountID, accountID)
	}

	// Fetch alias and verify it persists
	fetchedAlias, err := h.GetAlias(ctx, crm, crmHeaders, holderID, createdAlias.ID)
	if err != nil {
		t.Fatalf("GET alias failed: %v", err)
	}

	if fetchedAlias.ID != createdAlias.ID {
		t.Errorf("fetched alias ID mismatch: got %q, want %q", fetchedAlias.ID, createdAlias.ID)
	}

	// Cleanup
	if err := h.DeleteAlias(ctx, crm, crmHeaders, holderID, createdAlias.ID); err != nil {
		t.Logf("Warning: cleanup delete alias failed: %v", err)
	}
	if err := h.DeleteHolder(ctx, crm, crmHeaders, holderID); err != nil {
		t.Logf("Warning: cleanup delete holder failed: %v", err)
	}

	t.Log("Alias with banking details test completed successfully")
}
