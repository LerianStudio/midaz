package repl

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
)

// nopReadCloser wraps a Reader to implement ReadCloser
type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error { return nil }

func TestNew(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	config := &Config{}

	repl, err := New(f, rootCmd, config)
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	if repl == nil {
		t.Fatal("New should not return nil")
	}
	if repl.factory != f {
		t.Error("REPL should store factory reference")
	}
	if repl.context == nil {
		t.Error("REPL should have context")
	}
}

func TestREPL_parseCommandLine(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{}},
		{"  ", []string{}},
		{"single", []string{"single"}},
		{"multiple words", []string{"multiple", "words"}},
		{"word  with   spaces", []string{"word", "with", "spaces"}},
		{"\"quoted string\"", []string{"quoted string"}},
		{"'single quoted'", []string{"single quoted"}},
		{"mixed \"quoted string\" and normal", []string{"mixed", "quoted string", "and", "normal"}},
	}

	for _, test := range tests {
		result := parseCommandLine(test.input)
		if !equalSlices(result, test.expected) {
			t.Errorf("parseCommandLine(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

func TestREPL_buildWelcomeMessage(t *testing.T) {
	msg := buildWelcomeMessage()

	// Check that welcome message contains expected elements
	expectedElements := []string{
		"MDZ Interactive Mode",
		"Smart context-aware",
		"Quick Start",
		"help",
	}

	for _, element := range expectedElements {
		if !strings.Contains(msg, element) {
			t.Errorf("Welcome message should contain '%s'", element)
		}
	}
}

func TestREPL_executeCommand_Context(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test context command
	err = repl.executeCommand(ctx, "context")
	if err != nil {
		t.Errorf("Context command should not error: %v", err)
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "No context set") {
		t.Error("Context command should show 'No context set' for empty context")
	}
}

func TestREPL_executeCommand_Help(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test help command
	err = repl.executeCommand(ctx, "help")
	if err != nil {
		t.Errorf("Help command should not error: %v", err)
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "MDZ Interactive Help") {
		t.Error("Help command should show help content")
	}
}

func TestREPL_Context_Integration(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test use organization command
	err = repl.executeCommand(ctx, "use organization test-org-123")
	if err != nil {
		t.Errorf("Use organization command should not error: %v", err)
	}

	// Check that context was set
	if repl.context.OrganizationID != "test-org-123" {
		t.Errorf("Expected organization ID to be 'test-org-123', got '%s'", repl.context.OrganizationID)
	}

	// Test context display after setting organization
	f.IOStreams.Out = &bytes.Buffer{} // Reset output buffer
	err = repl.executeCommand(ctx, "context")
	if err != nil {
		t.Errorf("Context command should not error: %v", err)
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "test-org-123") {
		t.Error("Context should display organization ID")
	}
}

func TestREPL_GetContext(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	context := repl.GetContext()
	if context == nil {
		t.Error("GetContext should return non-nil context")
	}
}

// Helper functions
func createTestFactory() *factory.Factory {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}

	iostreams := &iostreams.IOStreams{
		In:  &nopReadCloser{strings.NewReader("")},
		Out: out,
		Err: err,
	}

	return &factory.Factory{
		IOStreams: iostreams,
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig should not return nil")
	}
	if config.Prompt != "mdz> " {
		t.Errorf("Expected default prompt 'mdz> ', got '%s'", config.Prompt)
	}
	if config.MaxHistory != 1000 {
		t.Errorf("Expected MaxHistory 1000, got %d", config.MaxHistory)
	}
	if len(config.ExitCommands) == 0 {
		t.Error("Default config should have exit commands")
	}

	// Check exit commands
	expectedExitCommands := []string{"exit", "quit", "q"}
	for _, cmd := range expectedExitCommands {
		if !contains(config.ExitCommands, cmd) {
			t.Errorf("Default config should include exit command '%s'", cmd)
		}
	}

	// Check welcome message
	if config.WelcomeMsg == "" {
		t.Error("Default config should have welcome message")
	}
}

func TestREPL_executeCommand_BuiltInCommands(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test history command
	repl.history = []string{"test command", "another command"}
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "history")
	if err != nil {
		t.Errorf("History command should not error: %v", err)
	}
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "test command") {
		t.Error("History should display previous commands")
	}

	// Test clear command
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "clear")
	if err != nil {
		t.Errorf("Clear command should not error: %v", err)
	}
	output = f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "\033[2J\033[H") {
		t.Error("Clear should output ANSI clear sequence")
	}

	// Test pwd command
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "pwd")
	if err != nil {
		t.Errorf("PWD command should not error: %v", err)
	}
	output = f.IOStreams.Out.(*bytes.Buffer).String()
	if output == "" {
		t.Error("PWD should output current directory")
	}

	// Test status command
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "status")
	if err != nil {
		t.Errorf("Status command should not error: %v", err)
	}
	output = f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "No context set") {
		t.Error("Status should show context information")
	}

	// Test st alias
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "st")
	if err != nil {
		t.Errorf("St command should not error: %v", err)
	}
}

func TestREPL_executeCommand_UseCommand(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test use command with insufficient args
	f.IOStreams.Err = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "use organization")
	if err != nil {
		t.Errorf("Use command with insufficient args should not error: %v", err)
	}
	errorOutput := f.IOStreams.Err.(*bytes.Buffer).String()
	if !strings.Contains(errorOutput, "Usage: use <entity> <id>") {
		t.Error("Should show usage message for insufficient args")
	}

	// Test valid use organization command
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "use organization test-org-123")
	if err != nil {
		t.Errorf("Use organization command should not error: %v", err)
	}
	if repl.context.OrganizationID != "test-org-123" {
		t.Error("Use command should set organization context")
	}

	// Test use ledger without organization
	f.IOStreams.Err = &bytes.Buffer{}
	repl.context.Clear()
	err = repl.executeCommand(ctx, "use ledger test-ledger-456")
	if err != nil {
		t.Errorf("Use ledger command should not error: %v", err)
	}
	errorOutput = f.IOStreams.Err.(*bytes.Buffer).String()
	if !strings.Contains(errorOutput, "No organization selected") {
		t.Error("Should warn about missing organization context")
	}

	// Test use with invalid entity type
	f.IOStreams.Err = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "use invalid test-id")
	if err != nil {
		t.Errorf("Use invalid command should not error: %v", err)
	}
	errorOutput = f.IOStreams.Err.(*bytes.Buffer).String()
	if !strings.Contains(errorOutput, "Unknown entity type") {
		t.Error("Should show error for unknown entity type")
	}
}

func TestREPL_executeCommand_UnsetCommand(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Set up full context
	repl.context.SetOrganization("org-123", "Test Org")
	repl.context.SetLedger("ledger-456", "Test Ledger")
	repl.context.SetPortfolio("portfolio-789", "Test Portfolio")
	repl.context.SetAccount("account-999", "Test Account")

	// Test unset with no args (clear all)
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "unset")
	if err != nil {
		t.Errorf("Unset command should not error: %v", err)
	}
	if repl.context.OrganizationID != "" {
		t.Error("Unset with no args should clear all context")
	}

	// Set up context again for specific unset tests
	repl.context.SetOrganization("org-123", "Test Org")
	repl.context.SetLedger("ledger-456", "Test Ledger")
	repl.context.SetPortfolio("portfolio-789", "Test Portfolio")
	repl.context.SetAccount("account-999", "Test Account")

	// Test unset account
	err = repl.executeCommand(ctx, "unset account")
	if err != nil {
		t.Errorf("Unset account should not error: %v", err)
	}
	if repl.context.AccountID != "" {
		t.Error("Unset account should clear account context")
	}
	if repl.context.PortfolioID == "" {
		t.Error("Unset account should preserve portfolio context")
	}

	// Test unset with invalid entity
	f.IOStreams.Err = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "unset invalid")
	if err != nil {
		t.Errorf("Unset invalid should not error: %v", err)
	}
	errorOutput := f.IOStreams.Err.(*bytes.Buffer).String()
	if !strings.Contains(errorOutput, "Unknown entity type") {
		t.Error("Should show error for unknown entity type")
	}
}

func TestREPL_executeCommand_SuggestionsCommand(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test suggestions command
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "suggestions")
	if err != nil {
		t.Errorf("Suggestions command should not error: %v", err)
	}
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Suggested Next Steps") {
		t.Error("Suggestions should show suggestions header")
	}

	// Test suggest alias
	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.executeCommand(ctx, "suggest")
	if err != nil {
		t.Errorf("Suggest command should not error: %v", err)
	}
}

func TestREPL_executeCommand_SmartList(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test ls command (should try to list organizations when no context)
	// This will fail with API error but tests the flow
	err = repl.executeCommand(ctx, "ls")
	if err == nil {
		t.Error("Expected error when no API setup for ls command")
	}

	// Test list alias
	err = repl.executeCommand(ctx, "list")
	if err == nil {
		t.Error("Expected error when no API setup for list command")
	}
}

func TestREPL_parseCommandLine_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Only spaces",
			input:    "   ",
			expected: []string{},
		},
		{
			name:     "Quoted with spaces inside",
			input:    `"hello world" test`,
			expected: []string{"hello world", "test"},
		},
		{
			name:     "Mixed quotes",
			input:    `'single' "double" normal`,
			expected: []string{"single", "double", "normal"},
		},
		{
			name:     "Escape sequences",
			input:    `test\nwith\\backslash`,
			expected: []string{"test\nwith\\backslash"},
		},
		{
			name:     "Quoted strings with escapes",
			input:    `"test\"quote"`,
			expected: []string{`test"quote`},
		},
		{
			name:     "Multiple spaces between args",
			input:    "arg1    arg2     arg3",
			expected: []string{"arg1", "arg2", "arg3"},
		},
		{
			name:     "Tab characters",
			input:    "arg1\targ2",
			expected: []string{"arg1\targ2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseCommandLine(test.input)
			if !equalSlices(result, test.expected) {
				t.Errorf("parseCommandLine(%q) = %v, expected %v", test.input, result, test.expected)
			}
		})
	}
}

func TestREPL_showHistory(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	// Add some history
	repl.history = []string{"command1", "command2", "command3"}

	err = repl.showHistory()
	if err != nil {
		t.Errorf("showHistory should not error: %v", err)
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	expectedItems := []string{"1  command1", "2  command2", "3  command3"}
	for _, item := range expectedItems {
		if !strings.Contains(output, item) {
			t.Errorf("History output should contain '%s', got: %s", item, output)
		}
	}
}

func TestREPL_clearScreen(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	err = repl.clearScreen()
	if err != nil {
		t.Errorf("clearScreen should not error: %v", err)
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "\033[2J\033[H") {
		t.Error("clearScreen should output ANSI clear sequence")
	}
}

func TestREPL_Close(t *testing.T) {
	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	err = repl.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestREPL_showContextualHelp_EdgeCases(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	// Test help with different context levels
	contexts := []struct {
		name         string
		setupFunc    func()
		expectedText string
	}{
		{
			name:         "No context",
			setupFunc:    func() {},
			expectedText: "organization list",
		},
		{
			name: "Organization context",
			setupFunc: func() {
				repl.context.SetOrganization("org-123", "Test Org")
			},
			expectedText: "ledger list",
		},
		{
			name: "Ledger context",
			setupFunc: func() {
				repl.context.SetOrganization("org-123", "Test Org")
				repl.context.SetLedger("ledger-456", "Test Ledger")
			},
			expectedText: "account list",
		},
		{
			name: "Account context",
			setupFunc: func() {
				repl.context.SetOrganization("org-123", "Test Org")
				repl.context.SetLedger("ledger-456", "Test Ledger")
				repl.context.SetAccount("account-999", "Test Account")
			},
			expectedText: "balance list",
		},
	}

	for _, ctx := range contexts {
		t.Run(ctx.name, func(t *testing.T) {
			repl.context.Clear()
			ctx.setupFunc()

			f.IOStreams.Out = &bytes.Buffer{}
			err := repl.showContextualHelp()
			if err != nil {
				t.Errorf("showContextualHelp should not error: %v", err)
			}

			output := f.IOStreams.Out.(*bytes.Buffer).String()
			if !strings.Contains(output, ctx.expectedText) {
				t.Errorf("Help for %s context should contain '%s', got: %s", ctx.name, ctx.expectedText, output)
			}
		})
	}
}

func TestREPL_showSuggestions_EdgeCases(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}

	// Test with portfolio context but no account
	repl.context.SetOrganization("org-123", "Test Org")
	repl.context.SetLedger("ledger-456", "Test Ledger")
	repl.context.SetPortfolio("portfolio-789", "Test Portfolio")

	f.IOStreams.Out = &bytes.Buffer{}
	err = repl.showSuggestions()
	if err != nil {
		t.Errorf("showSuggestions should not error: %v", err)
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "You have context set up") {
		t.Error("Should show generic message for partial context")
	}
}

func TestREPL_handleSmartList_EdgeCases(t *testing.T) {
	clearREPLTestEnvironment()
	defer clearREPLTestEnvironment()

	f := createTestFactory()
	rootCmd := &cobra.Command{Use: "mdz"}
	repl, err := New(f, rootCmd, &Config{})
	if err != nil {
		t.Fatalf("New should not return error: %v", err)
	}
	ctx := context.Background()

	// Test smart list with account context (should list balances)
	repl.context.SetOrganization("org-123", "Test Org")
	repl.context.SetLedger("ledger-456", "Test Ledger")
	repl.context.SetAccount("account-999", "Test Account")

	// This will fail with API error but tests the logic
	err = repl.handleSmartList(ctx)
	if err == nil {
		t.Error("Expected error when no API setup")
	}

	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Listing balances") {
		t.Error("Should indicate listing balances for account context")
	}
}

func TestREPL_createCompleter(t *testing.T) {
	rootCmd := &cobra.Command{Use: "mdz"}

	// Add some subcommands
	orgCmd := &cobra.Command{Use: "organization"}
	orgCmd.AddCommand(&cobra.Command{Use: "list"})
	orgCmd.AddCommand(&cobra.Command{Use: "create"})
	rootCmd.AddCommand(orgCmd)

	completer := createCompleter(rootCmd)
	if completer == nil {
		t.Fatal("createCompleter should not return nil")
	}
}

func TestREPL_buildCommandCompleter(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("flag1", "", "Test flag")
	cmd.Flags().StringP("flag2", "f", "", "Test flag with shorthand")

	subCmd := &cobra.Command{Use: "subcmd"}
	cmd.AddCommand(subCmd)

	completer := buildCommandCompleter(cmd)
	if completer == nil {
		t.Fatal("buildCommandCompleter should not return nil")
	}
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func clearREPLTestEnvironment() {
	envVars := []string{
		"MDZ_CONTEXT_ORG_ID", "MDZ_CONTEXT_ORG_NAME",
		"MDZ_CONTEXT_LEDGER_ID", "MDZ_CONTEXT_LEDGER_NAME",
		"MDZ_CONTEXT_PORTFOLIO_ID", "MDZ_CONTEXT_PORTFOLIO_NAME",
		"MDZ_CONTEXT_ACCOUNT_ID", "MDZ_CONTEXT_ACCOUNT_NAME",
		"MDZ_CONTEXT_UPDATED", "MDZ_REPL_MODE",
	}

	for _, env := range envVars {
		os.Unsetenv(env)
	}
}
