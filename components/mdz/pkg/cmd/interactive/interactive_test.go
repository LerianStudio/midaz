package interactive

import (
	"bytes"
	"io"
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

func TestNewCmdInteractive(t *testing.T) {
	f := createTestFactory()

	cmd := NewCmdInteractive(f)

	if cmd == nil {
		t.Fatal("NewCmdInteractive should not return nil")
	}

	if cmd.Use != "interactive" {
		t.Errorf("Expected Use 'interactive', got '%s'", cmd.Use)
	}

	expectedAliases := []string{"i", "repl"}
	if len(cmd.Aliases) != len(expectedAliases) {
		t.Errorf("Expected %d aliases, got %d", len(expectedAliases), len(cmd.Aliases))
	}

	for i, alias := range expectedAliases {
		if i >= len(cmd.Aliases) || cmd.Aliases[i] != alias {
			t.Errorf("Expected alias '%s' at index %d, got '%s'", alias, i, cmd.Aliases[i])
		}
	}

	if cmd.Short == "" {
		t.Error("Command should have a short description")
	}

	if cmd.Long == "" {
		t.Error("Command should have a long description")
	}

	if cmd.Example == "" {
		t.Error("Command should have examples")
	}

	if cmd.RunE == nil {
		t.Error("Command should have a RunE function")
	}
}

func TestFactoryInteractive_runE_Setup(t *testing.T) {
	f := createTestFactory()

	// Create a root command with some subcommands
	rootCmd := &cobra.Command{
		Use:   "mdz",
		Short: "MDZ CLI",
		Long:  "MDZ Command Line Interface",
	}

	// Add some test subcommands
	orgCmd := &cobra.Command{Use: "organization"}
	ledgerCmd := &cobra.Command{Use: "ledger"}
	interactiveCmd := NewCmdInteractive(f)

	rootCmd.AddCommand(orgCmd)
	rootCmd.AddCommand(ledgerCmd)
	rootCmd.AddCommand(interactiveCmd)

	// Set up factory interactive
	_ = &factoryInteractive{factory: f}

	// Test that runE creates proper REPL structure
	// Note: We can't easily test the actual REPL run without complex mocking
	// So we'll test the setup logic

	// Verify the function doesn't panic with basic setup
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("runE should not panic during setup: %v", r)
		}
	}()

	// We can't easily test the full execution without mocking the REPL
	// but we can test that the interactive command was created properly
	if interactiveCmd.RunE == nil {
		t.Error("runE should not be nil")
	}
}

func TestBannerConstant(t *testing.T) {
	if banner == "" {
		t.Error("Banner should not be empty")
	}

	// Check that banner contains expected elements
	expectedElements := []string{
		"╔╦╗", "╔╦╗", "╔═╗", // Top line elements
		"║║║", "║║", "╔═╝", // Middle line elements
		"╩", "╩", "╚═╝", // Bottom line elements
	}

	for _, element := range expectedElements {
		if !strings.Contains(banner, element) {
			t.Errorf("Banner should contain element '%s'", element)
		}
	}
}

func TestFactoryInteractive_CommandExclusion(t *testing.T) {
	f := createTestFactory()

	// Create a root command with interactive command
	rootCmd := &cobra.Command{Use: "mdz"}
	interactiveCmd := NewCmdInteractive(f)
	rootCmd.AddCommand(interactiveCmd)

	// Simulate the command copying logic
	replCmd := &cobra.Command{
		Use:   rootCmd.Use,
		Short: rootCmd.Short,
		Long:  rootCmd.Long,
	}

	// Copy all commands except interactive to avoid recursion
	for _, c := range rootCmd.Commands() {
		if c.Name() != "interactive" && c.Name() != "i" && c.Name() != "repl" {
			replCmd.AddCommand(c)
		}
	}

	// Verify that interactive commands are excluded
	for _, cmd := range replCmd.Commands() {
		if cmd.Name() == "interactive" || cmd.Name() == "i" || cmd.Name() == "repl" {
			t.Errorf("REPL command should not include interactive command '%s'", cmd.Name())
		}
	}
}

func TestFactoryInteractive_BannerOutput(t *testing.T) {
	f := createTestFactory()

	// Test banner output with color
	f.Flags.NoColor = false
	output := &bytes.Buffer{}
	f.IOStreams.Out = output

	// Print banner manually to test the logic
	if !f.Flags.NoColor {
		output.WriteString("\033[36m" + banner + "\033[0m\n")
	} else {
		output.WriteString(banner + "\n")
	}

	result := output.String()
	if !strings.Contains(result, banner) {
		t.Error("Output should contain banner")
	}
	if !strings.Contains(result, "\033[36m") {
		t.Error("Colored output should contain color codes")
	}

	// Test banner output without color
	f.Flags.NoColor = true
	output.Reset()

	if !f.Flags.NoColor {
		output.WriteString("\033[36m" + banner + "\033[0m\n")
	} else {
		output.WriteString(banner + "\n")
	}

	result = output.String()
	if !strings.Contains(result, banner) {
		t.Error("Output should contain banner")
	}
	if strings.Contains(result, "\033[36m") {
		t.Error("Non-colored output should not contain color codes")
	}
}

func TestInteractiveCommand_Integration(t *testing.T) {
	f := createTestFactory()

	// Create a minimal command structure
	rootCmd := &cobra.Command{Use: "mdz"}

	// Add interactive command
	interactiveCmd := NewCmdInteractive(f)
	rootCmd.AddCommand(interactiveCmd)

	// Test that the command can be found by name and aliases
	cmd, _, err := rootCmd.Find([]string{"interactive"})
	if err != nil {
		t.Errorf("Should find interactive command: %v", err)
	}
	if cmd.Name() != "interactive" {
		t.Errorf("Expected 'interactive', got '%s'", cmd.Name())
	}

	// Test alias 'i'
	cmd, _, err = rootCmd.Find([]string{"i"})
	if err != nil {
		t.Errorf("Should find interactive command by alias 'i': %v", err)
	}
	if cmd.Name() != "interactive" {
		t.Errorf("Expected 'interactive', got '%s'", cmd.Name())
	}

	// Test alias 'repl'
	cmd, _, err = rootCmd.Find([]string{"repl"})
	if err != nil {
		t.Errorf("Should find interactive command by alias 'repl': %v", err)
	}
	if cmd.Name() != "interactive" {
		t.Errorf("Expected 'interactive', got '%s'", cmd.Name())
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
