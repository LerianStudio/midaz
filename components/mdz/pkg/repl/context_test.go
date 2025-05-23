package repl

import (
	"os"
	"strings"
	"testing"
)

func TestNewContext(t *testing.T) {
	// Clear environment before test
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()
	if ctx == nil {
		t.Fatal("NewContext should not return nil")
	}

	// Should be empty initially
	if ctx.OrganizationID != "" || ctx.LedgerID != "" {
		t.Error("New context should be empty")
	}
}

func TestContext_SetOrganization(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	orgID := "test-org-123"
	orgName := "Test Organization"

	ctx.SetOrganization(orgID, orgName)

	// Check context values
	if ctx.OrganizationID != orgID {
		t.Errorf("Expected OrganizationID %s, got %s", orgID, ctx.OrganizationID)
	}
	if ctx.OrganizationName != orgName {
		t.Errorf("Expected OrganizationName %s, got %s", orgName, ctx.OrganizationName)
	}

	// Check environment variables
	if os.Getenv("MDZ_CONTEXT_ORG_ID") != orgID {
		t.Error("Environment variable MDZ_CONTEXT_ORG_ID not set correctly")
	}
	if os.Getenv("MDZ_CONTEXT_ORG_NAME") != orgName {
		t.Error("Environment variable MDZ_CONTEXT_ORG_NAME not set correctly")
	}

	// Should clear dependent context
	if ctx.LedgerID != "" || ctx.PortfolioID != "" || ctx.AccountID != "" {
		t.Error("Setting organization should clear dependent context")
	}
}

func TestContext_SetLedger(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	// Set up parent context first
	ctx.SetOrganization("org-123", "Test Org")

	ledgerID := "ledger-456"
	ledgerName := "Test Ledger"

	ctx.SetLedger(ledgerID, ledgerName)

	// Check context values
	if ctx.LedgerID != ledgerID {
		t.Errorf("Expected LedgerID %s, got %s", ledgerID, ctx.LedgerID)
	}
	if ctx.LedgerName != ledgerName {
		t.Errorf("Expected LedgerName %s, got %s", ledgerName, ctx.LedgerName)
	}

	// Check environment variables
	if os.Getenv("MDZ_CONTEXT_LEDGER_ID") != ledgerID {
		t.Error("Environment variable MDZ_CONTEXT_LEDGER_ID not set correctly")
	}

	// Should preserve parent context
	if ctx.OrganizationID != "org-123" {
		t.Error("Setting ledger should preserve organization context")
	}

	// Should clear dependent context
	if ctx.PortfolioID != "" || ctx.AccountID != "" {
		t.Error("Setting ledger should clear dependent context")
	}
}

func TestContext_loadFromEnvironment(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	// Set up environment variables
	os.Setenv("MDZ_CONTEXT_ORG_ID", "env-org-123")
	os.Setenv("MDZ_CONTEXT_ORG_NAME", "Env Organization")
	os.Setenv("MDZ_CONTEXT_LEDGER_ID", "env-ledger-456")
	os.Setenv("MDZ_CONTEXT_LEDGER_NAME", "Env Ledger")

	ctx := &Context{}
	ctx.loadFromEnvironment()

	// Check that values were loaded from environment
	if ctx.OrganizationID != "env-org-123" {
		t.Errorf("Expected OrganizationID from env, got %s", ctx.OrganizationID)
	}
	if ctx.OrganizationName != "Env Organization" {
		t.Errorf("Expected OrganizationName from env, got %s", ctx.OrganizationName)
	}
	if ctx.LedgerID != "env-ledger-456" {
		t.Errorf("Expected LedgerID from env, got %s", ctx.LedgerID)
	}
	if ctx.LedgerName != "Env Ledger" {
		t.Errorf("Expected LedgerName from env, got %s", ctx.LedgerName)
	}
}

func TestContext_Clear(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	// Set up full context
	ctx.SetOrganization("org-123", "Test Org")
	ctx.SetLedger("ledger-456", "Test Ledger")
	ctx.SetPortfolio("portfolio-789", "Test Portfolio")
	ctx.SetAccount("account-999", "Test Account")

	// Clear all context
	ctx.Clear()

	// Check that all context is cleared
	if ctx.OrganizationID != "" || ctx.LedgerID != "" ||
		ctx.PortfolioID != "" || ctx.AccountID != "" {
		t.Error("Clear should remove all context")
	}

	// Check that environment variables are cleared
	if os.Getenv("MDZ_CONTEXT_ORG_ID") != "" {
		t.Error("Clear should remove environment variables")
	}
}

func TestContext_GetPrompt(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	// Empty context
	prompt := ctx.GetPrompt()
	if prompt != "mdz> " {
		t.Errorf("Expected 'mdz> ', got '%s'", prompt)
	}

	// With organization
	ctx.SetOrganization("org-123", "Test Org")
	prompt = ctx.GetPrompt()
	if prompt != "mdz [Test Org]> " {
		t.Errorf("Expected 'mdz [Test Org]> ', got '%s'", prompt)
	}

	// With organization and ledger
	ctx.SetLedger("ledger-456", "Test Ledger")
	prompt = ctx.GetPrompt()
	if prompt != "mdz [Test Org/Test Ledger]> " {
		t.Errorf("Expected 'mdz [Test Org/Test Ledger]> ', got '%s'", prompt)
	}
}

func TestContext_String(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	// Empty context should show getting started message
	str := ctx.String()
	if !containsSubstring(str, "No context set") {
		t.Error("Empty context should show 'No context set' message")
	}

	// With context should show formatted details
	ctx.SetOrganization("org-123", "Test Organization")
	str = ctx.String()
	if !containsSubstring(str, "Test Organization") {
		t.Error("Context string should include organization name")
	}
	if !containsSubstring(str, "org-123") {
		t.Error("Context string should include organization ID")
	}
}

func TestContext_SetPortfolio(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()
	ctx.SetOrganization("org-123", "Test Org")
	ctx.SetLedger("ledger-456", "Test Ledger")

	portfolioID := "portfolio-789"
	portfolioName := "Test Portfolio"

	ctx.SetPortfolio(portfolioID, portfolioName)

	if ctx.PortfolioID != portfolioID {
		t.Errorf("Expected PortfolioID %s, got %s", portfolioID, ctx.PortfolioID)
	}
	if ctx.PortfolioName != portfolioName {
		t.Errorf("Expected PortfolioName %s, got %s", portfolioName, ctx.PortfolioName)
	}

	// Should clear dependent context
	if ctx.AccountID != "" {
		t.Error("Setting portfolio should clear account context")
	}

	// Should preserve parent context
	if ctx.OrganizationID != "org-123" || ctx.LedgerID != "ledger-456" {
		t.Error("Setting portfolio should preserve parent context")
	}
}

func TestContext_SetAccount(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()
	ctx.SetOrganization("org-123", "Test Org")
	ctx.SetLedger("ledger-456", "Test Ledger")
	ctx.SetPortfolio("portfolio-789", "Test Portfolio")

	accountID := "account-999"
	accountName := "Test Account"

	ctx.SetAccount(accountID, accountName)

	if ctx.AccountID != accountID {
		t.Errorf("Expected AccountID %s, got %s", accountID, ctx.AccountID)
	}
	if ctx.AccountName != accountName {
		t.Errorf("Expected AccountName %s, got %s", accountName, ctx.AccountName)
	}

	// Should preserve all parent context
	if ctx.OrganizationID != "org-123" || ctx.LedgerID != "ledger-456" || ctx.PortfolioID != "portfolio-789" {
		t.Error("Setting account should preserve parent context")
	}
}

func TestContext_ClearLedger(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()
	ctx.SetOrganization("org-123", "Test Org")
	ctx.SetLedger("ledger-456", "Test Ledger")
	ctx.SetPortfolio("portfolio-789", "Test Portfolio")
	ctx.SetAccount("account-999", "Test Account")

	ctx.ClearLedger()

	// Should clear ledger and dependent context
	if ctx.LedgerID != "" || ctx.PortfolioID != "" || ctx.AccountID != "" {
		t.Error("ClearLedger should clear ledger and dependent context")
	}

	// Should preserve organization context
	if ctx.OrganizationID != "org-123" {
		t.Error("ClearLedger should preserve organization context")
	}
}

func TestContext_ClearPortfolio(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()
	ctx.SetOrganization("org-123", "Test Org")
	ctx.SetLedger("ledger-456", "Test Ledger")
	ctx.SetPortfolio("portfolio-789", "Test Portfolio")
	ctx.SetAccount("account-999", "Test Account")

	ctx.ClearPortfolio()

	// Should clear portfolio and account context
	if ctx.PortfolioID != "" || ctx.AccountID != "" {
		t.Error("ClearPortfolio should clear portfolio and account context")
	}

	// Should preserve organization and ledger context
	if ctx.OrganizationID != "org-123" || ctx.LedgerID != "ledger-456" {
		t.Error("ClearPortfolio should preserve organization and ledger context")
	}
}

func TestContext_ClearAccount(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()
	ctx.SetOrganization("org-123", "Test Org")
	ctx.SetLedger("ledger-456", "Test Ledger")
	ctx.SetPortfolio("portfolio-789", "Test Portfolio")
	ctx.SetAccount("account-999", "Test Account")

	ctx.ClearAccount()

	// Should clear only account context
	if ctx.AccountID != "" {
		t.Error("ClearAccount should clear account context")
	}

	// Should preserve all parent context
	if ctx.OrganizationID != "org-123" || ctx.LedgerID != "ledger-456" || ctx.PortfolioID != "portfolio-789" {
		t.Error("ClearAccount should preserve parent context")
	}
}

func TestContext_GetPrompt_EdgeCases(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	// Test with only IDs (no names)
	ctx.OrganizationID = "org-123456789012345678901234567890"
	prompt := ctx.GetPrompt()
	if !strings.Contains(prompt, "org:org-1234") {
		t.Errorf("Expected prompt to contain truncated org ID, got '%s'", prompt)
	}

	// Test mixed names and IDs
	ctx.OrganizationName = "Test Org"
	ctx.LedgerID = "ledger-123456789012345678901234567890"
	prompt = ctx.GetPrompt()
	expected := "mdz [Test Org/led:ledger-1]> "
	if prompt != expected {
		t.Errorf("Expected prompt '%s', got '%s'", expected, prompt)
	}
}

func TestContext_String_EdgeCases(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	ctx := NewContext()

	// Test with unnamed entities
	ctx.OrganizationID = "org-123"
	str := ctx.String()
	if !strings.Contains(str, "Unnamed") {
		t.Error("Context string should show 'Unnamed' for entities without names")
	}

	// Test full context hierarchy
	ctx.OrganizationName = "Test Organization"
	ctx.LedgerID = "ledger-456"
	ctx.LedgerName = "Test Ledger"
	ctx.PortfolioID = "portfolio-789"
	ctx.PortfolioName = "Test Portfolio"
	ctx.AccountID = "account-999"
	ctx.AccountName = "Test Account"

	str = ctx.String()

	// Should contain all entities
	entities := []string{"Test Organization", "Test Ledger", "Test Portfolio", "Test Account"}
	for _, entity := range entities {
		if !strings.Contains(str, entity) {
			t.Errorf("Context string should contain '%s'", entity)
		}
	}

	// Should contain appropriate suggestions for account context
	if !strings.Contains(str, "balance list") {
		t.Error("Context string should suggest balance list for account context")
	}
}

func TestContext_loadFromEnvironment_PartialData(t *testing.T) {
	clearTestEnvironment()
	defer clearTestEnvironment()

	// Set partial environment data
	os.Setenv("MDZ_CONTEXT_ORG_ID", "env-org-123")
	// Intentionally not setting ORG_NAME
	os.Setenv("MDZ_CONTEXT_PORTFOLIO_ID", "env-portfolio-789")
	os.Setenv("MDZ_CONTEXT_PORTFOLIO_NAME", "Env Portfolio")
	// Set update flag
	os.Setenv("MDZ_CONTEXT_UPDATED", "true")

	ctx := &Context{}
	ctx.loadFromEnvironment()

	// Should load what's available
	if ctx.OrganizationID != "env-org-123" {
		t.Errorf("Expected OrganizationID from env, got %s", ctx.OrganizationID)
	}
	if ctx.OrganizationName != "" {
		t.Errorf("Expected empty OrganizationName, got %s", ctx.OrganizationName)
	}
	if ctx.PortfolioID != "env-portfolio-789" {
		t.Errorf("Expected PortfolioID from env, got %s", ctx.PortfolioID)
	}

	// Should clear the update flag
	if os.Getenv("MDZ_CONTEXT_UPDATED") != "" {
		t.Error("loadFromEnvironment should clear MDZ_CONTEXT_UPDATED flag")
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"very-long-uuid-12345678901234567890", "very-lon"},
	}

	for _, test := range tests {
		result := truncateID(test.input)
		if result != test.expected {
			t.Errorf("truncateID(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

// Helper functions for tests
func clearTestEnvironment() {
	envVars := []string{
		"MDZ_CONTEXT_ORG_ID", "MDZ_CONTEXT_ORG_NAME",
		"MDZ_CONTEXT_LEDGER_ID", "MDZ_CONTEXT_LEDGER_NAME",
		"MDZ_CONTEXT_PORTFOLIO_ID", "MDZ_CONTEXT_PORTFOLIO_NAME",
		"MDZ_CONTEXT_ACCOUNT_ID", "MDZ_CONTEXT_ACCOUNT_NAME",
		"MDZ_CONTEXT_UPDATED",
	}

	for _, env := range envVars {
		os.Unsetenv(env)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsStringContext(s, substr))))
}

func containsStringContext(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
