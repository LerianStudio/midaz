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
	headers := h.AuthHeaders(h.RandHex(8))

	// ─────────────────────────────────────────────────────────────────────────
	// SETUP: Create Organization → Ledger → Asset → Account
	// ─────────────────────────────────────────────────────────────────────────

	// Create Organization
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Alias Test Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}
	t.Logf("Created organization: ID=%s", orgID)

	// Create Ledger
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Alias Test Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}
	t.Logf("Created ledger: ID=%s", ledgerID)

	// Create USD Asset
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}
	t.Log("Created USD asset")

	// Create Account
	accountAlias := fmt.Sprintf("alias-test-account-%s", h.RandString(6))
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, accountAlias, "USD")
	if err != nil {
		t.Fatalf("setup account failed: %v", err)
	}
	t.Logf("Created account: ID=%s Alias=%s", accountID, accountAlias)

	// Create Holder
	holderName := fmt.Sprintf("Alias Test Holder %s", h.RandString(6))
	holderCPF := h.GenerateValidCPF()
	holderID, err := h.SetupHolder(ctx, crm, headers, holderName, holderCPF, "NATURAL_PERSON")
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
	code, body, err := crm.Request(ctx, "POST", path, headers, createPayload)
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
	fetchedAlias, err := h.GetAlias(ctx, crm, headers, holderID, aliasID)
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
	aliasList, err := h.ListAliases(ctx, crm, headers, holderID)
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
	allAliases, err := h.ListAllAliases(ctx, crm, headers)
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

	updatedAlias, err := h.UpdateAlias(ctx, crm, headers, holderID, aliasID, updatePayload)
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
	verifyAlias, err := h.GetAlias(ctx, crm, headers, holderID, aliasID)
	if err != nil {
		t.Fatalf("GET alias after update failed: %v", err)
	}
	if verifyAlias.ID != aliasID {
		t.Errorf("verified alias ID mismatch: got %q, want %q", verifyAlias.ID, aliasID)
	}

	// ─────────────────────────────────────────────────────────────────────────
	// 6) DELETE - Remove Alias
	// ─────────────────────────────────────────────────────────────────────────
	err = h.DeleteAlias(ctx, crm, headers, holderID, aliasID)
	if err != nil {
		t.Fatalf("DELETE alias failed: %v", err)
	}

	t.Logf("Deleted alias: ID=%s", aliasID)

	// Verify deletion - GET should fail
	_, err = h.GetAlias(ctx, crm, headers, holderID, aliasID)
	if err == nil {
		t.Errorf("GET deleted alias should fail, but succeeded")
	}

	// ─────────────────────────────────────────────────────────────────────────
	// CLEANUP
	// ─────────────────────────────────────────────────────────────────────────
	if err := h.DeleteHolder(ctx, crm, headers, holderID); err != nil {
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
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: Organization → Ledger → Asset → 2 Accounts
	orgID, err := h.SetupOrganization(ctx, onboard, headers, fmt.Sprintf("Multi Alias Org %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup organization failed: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, fmt.Sprintf("Multi Alias Ledger %s", h.RandString(5)))
	if err != nil {
		t.Fatalf("setup ledger failed: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset failed: %v", err)
	}

	// Create two accounts
	account1ID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, fmt.Sprintf("account1-%s", h.RandString(6)), "USD")
	if err != nil {
		t.Fatalf("setup account 1 failed: %v", err)
	}

	account2ID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, fmt.Sprintf("account2-%s", h.RandString(6)), "USD")
	if err != nil {
		t.Fatalf("setup account 2 failed: %v", err)
	}

	// Create Holder
	holderID, err := h.SetupHolder(ctx, crm, headers, fmt.Sprintf("Multi Alias Holder %s", h.RandString(6)), h.GenerateValidCPF(), "NATURAL_PERSON")
	if err != nil {
		t.Fatalf("setup holder failed: %v", err)
	}

	// Create first alias
	alias1ID, err := h.SetupAlias(ctx, crm, headers, holderID, ledgerID, account1ID)
	if err != nil {
		t.Fatalf("create alias 1 failed: %v", err)
	}
	t.Logf("Created alias 1: ID=%s for account %s", alias1ID, account1ID)

	// Create second alias
	alias2ID, err := h.SetupAlias(ctx, crm, headers, holderID, ledgerID, account2ID)
	if err != nil {
		t.Fatalf("create alias 2 failed: %v", err)
	}
	t.Logf("Created alias 2: ID=%s for account %s", alias2ID, account2ID)

	// List aliases for holder - should have both
	aliasList, err := h.ListAliases(ctx, crm, headers, holderID)
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
	_ = h.DeleteAlias(ctx, crm, headers, holderID, alias1ID)
	_ = h.DeleteAlias(ctx, crm, headers, holderID, alias2ID)
	_ = h.DeleteHolder(ctx, crm, headers, holderID)
}
