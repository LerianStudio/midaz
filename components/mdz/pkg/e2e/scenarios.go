package e2e

import "time"

// GetDefaultScenarios returns a set of predefined test scenarios for MDZ CLI
func GetDefaultScenarios() []*Scenario {
	return []*Scenario{
		getLoginScenario(),
		getOrganizationListScenario(),
		getLedgerManagementScenario(),
		getAccountCreationScenario(),
		getTransactionFlowScenario(),
		getREPLInteractiveScenario(),
		getErrorHandlingScenario(),
		getHelpSystemScenario(),
	}
}

// getLoginScenario tests the authentication flow
func getLoginScenario() *Scenario {
	return &Scenario{
		Name:        "login_flow",
		Description: "Test the complete login and authentication flow",
		Steps: []Step{
			{
				Type:        "wait_for_output",
				ExpectText:  "Welcome to MDZ",
				Timeout:     5 * time.Second,
				Description: "Wait for welcome message",
			},
			{
				Type:        "type",
				Input:       "login",
				Description: "Enter login command",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute login command",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "Username:",
				Timeout:     5 * time.Second,
				Description: "Wait for username prompt",
			},
			{
				Type:        "type",
				Input:       "testuser",
				Description: "Enter username",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Submit username",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "Password:",
				Timeout:     5 * time.Second,
				Description: "Wait for password prompt",
			},
			{
				Type:        "type",
				Input:       "testpass",
				Description: "Enter password",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Submit password",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Login successful",
				Timeout:     10 * time.Second,
				Description: "Wait for login confirmation",
			},
		},
		Expected: []string{
			"Login successful",
			"Authentication completed",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE": "true",
			},
		},
	}
}

// getOrganizationListScenario tests organization listing
func getOrganizationListScenario() *Scenario {
	return &Scenario{
		Name:        "organization_list",
		Description: "Test listing and selecting organizations",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "organization list",
				Description: "List all organizations",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute list command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Available organizations:",
				Timeout:     10 * time.Second,
				Description: "Wait for organization list",
			},
			{
				Type:        "screenshot",
				Description: "Capture organization list view",
			},
			{
				Type:        "type",
				Input:       "1",
				Description: "Select first organization",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Confirm selection",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Organization context set",
				Timeout:     5 * time.Second,
				Description: "Wait for context confirmation",
			},
		},
		Expected: []string{
			"Available organizations:",
			"Organization context set",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE": "true",
				"MDZ_API_URL":   "http://localhost:8080",
			},
		},
	}
}

// getLedgerManagementScenario tests ledger operations
func getLedgerManagementScenario() *Scenario {
	return &Scenario{
		Name:        "ledger_management",
		Description: "Test ledger creation, listing, and management",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "ledger create --name='Test Ledger' --description='E2E Test Ledger'",
				Description: "Create a new ledger",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute create command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Ledger created successfully",
				Timeout:     15 * time.Second,
				Description: "Wait for creation confirmation",
			},
			{
				Type:        "type",
				Input:       "ledger list",
				Description: "List all ledgers",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute list command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Test Ledger",
				Timeout:     10 * time.Second,
				Description: "Verify ledger appears in list",
			},
			{
				Type:        "screenshot",
				Description: "Capture ledger list",
			},
		},
		Expected: []string{
			"Ledger created successfully",
			"Test Ledger",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE":   "true",
				"MDZ_ORG_ID":      "test-org-123",
				"MDZ_ORG_NAME":    "Test Organization",
			},
		},
		Cleanup: &Cleanup{
			Commands: []string{
				"ledger delete --name='Test Ledger'",
			},
		},
	}
}

// getAccountCreationScenario tests account creation flow
func getAccountCreationScenario() *Scenario {
	return &Scenario{
		Name:        "account_creation",
		Description: "Test account creation with various input methods",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "account create",
				Description: "Start account creation",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute create command",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "Account name:",
				Timeout:     5 * time.Second,
				Description: "Wait for name prompt",
			},
			{
				Type:        "type",
				Input:       "Test Account",
				Description: "Enter account name",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Submit name",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "Account type:",
				Timeout:     5 * time.Second,
				Description: "Wait for type prompt",
			},
			{
				Type:        "type",
				Input:       "asset",
				Description: "Enter account type",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Submit type",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Account created",
				Timeout:     15 * time.Second,
				Description: "Wait for creation confirmation",
			},
		},
		Expected: []string{
			"Account created",
			"Test Account",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE":    "true",
				"MDZ_ORG_ID":       "test-org-123",
				"MDZ_LEDGER_ID":    "test-ledger-456",
				"MDZ_PORTFOLIO_ID": "test-portfolio-789",
			},
		},
	}
}

// getTransactionFlowScenario tests transaction creation
func getTransactionFlowScenario() *Scenario {
	return &Scenario{
		Name:        "transaction_flow",
		Description: "Test complete transaction creation flow",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "transaction create",
				Description: "Start transaction creation",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute create command",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "From account:",
				Timeout:     5 * time.Second,
				Description: "Wait for from account prompt",
			},
			{
				Type:        "type",
				Input:       "1",
				Description: "Select first account",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Confirm from account",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "To account:",
				Timeout:     5 * time.Second,
				Description: "Wait for to account prompt",
			},
			{
				Type:        "type",
				Input:       "2",
				Description: "Select second account",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Confirm to account",
			},
			{
				Type:        "wait_for_prompt",
				ExpectText:  "Amount:",
				Timeout:     5 * time.Second,
				Description: "Wait for amount prompt",
			},
			{
				Type:        "type",
				Input:       "100.00",
				Description: "Enter transaction amount",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Submit amount",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Transaction created",
				Timeout:     15 * time.Second,
				Description: "Wait for transaction confirmation",
			},
		},
		Expected: []string{
			"Transaction created",
			"100.00",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE":    "true",
				"MDZ_ORG_ID":       "test-org-123",
				"MDZ_LEDGER_ID":    "test-ledger-456",
				"MDZ_ACCOUNT_ID":   "test-account-789",
			},
		},
	}
}

// getREPLInteractiveScenario tests REPL mode
func getREPLInteractiveScenario() *Scenario {
	return &Scenario{
		Name:        "repl_interactive",
		Description: "Test REPL interactive mode and context switching",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "interactive",
				Description: "Enter REPL mode",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Start interactive session",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "MDZ Interactive Mode",
				Timeout:     5 * time.Second,
				Description: "Wait for REPL welcome",
			},
			{
				Type:        "type",
				Input:       "ls",
				Description: "Use smart list command",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute smart list",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Listing organizations",
				Timeout:     10 * time.Second,
				Description: "Wait for organization list",
			},
			{
				Type:        "type",
				Input:       "use organization 1",
				Description: "Set organization context",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute context command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Organization context set",
				Timeout:     5 * time.Second,
				Description: "Wait for context confirmation",
			},
			{
				Type:        "type",
				Input:       "context",
				Description: "Show current context",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute context command",
			},
			{
				Type:        "screenshot",
				Description: "Capture context view",
			},
			{
				Type:        "press",
				Key:         "ctrl+c",
				Description: "Exit REPL mode",
			},
		},
		Expected: []string{
			"MDZ Interactive Mode",
			"Organization context set",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE": "true",
			},
		},
	}
}

// getErrorHandlingScenario tests error scenarios
func getErrorHandlingScenario() *Scenario {
	return &Scenario{
		Name:        "error_handling",
		Description: "Test error handling and recovery scenarios",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "invalid-command",
				Description: "Enter invalid command",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute invalid command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Unknown command",
				Timeout:     5 * time.Second,
				Description: "Wait for error message",
			},
			{
				Type:        "type",
				Input:       "organization create",
				Description: "Try command without required args",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute incomplete command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "required",
				Timeout:     5 * time.Second,
				Description: "Wait for validation error",
			},
			{
				Type:        "type",
				Input:       "help organization create",
				Description: "Get help for command",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute help command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Usage:",
				Timeout:     5 * time.Second,
				Description: "Wait for help text",
			},
			{
				Type:        "screenshot",
				Description: "Capture help output",
			},
		},
		Expected: []string{
			"Unknown command",
			"Usage:",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE": "true",
			},
		},
	}
}

// getHelpSystemScenario tests help system functionality
func getHelpSystemScenario() *Scenario {
	return &Scenario{
		Name:        "help_system",
		Description: "Test help system discoverability and usefulness",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "help",
				Description: "Get general help",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute help command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Available commands:",
				Timeout:     5 * time.Second,
				Description: "Wait for command list",
			},
			{
				Type:        "type",
				Input:       "--help",
				Description: "Try help flag",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute help flag",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Usage:",
				Timeout:     5 * time.Second,
				Description: "Wait for usage info",
			},
			{
				Type:        "type",
				Input:       "organization --help",
				Description: "Get command-specific help",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute command help",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "organization",
				Timeout:     5 * time.Second,
				Description: "Wait for command help",
			},
			{
				Type:        "screenshot",
				Description: "Capture help system view",
			},
		},
		Expected: []string{
			"Available commands:",
			"Usage:",
		},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE": "true",
			},
		},
	}
}