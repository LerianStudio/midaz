package utils

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

// InteractiveSelector represents an entity that can be selected interactively
type InteractiveSelector struct {
	ID          string
	Name        string
	Description string
	Type        string
}

// IsInREPL detects if the command is being run within the REPL
func IsInREPL() bool {
	// Check if stdin is a terminal and there are specific environment markers
	// This is a heuristic - in production, you might want a more reliable method
	return os.Getenv("MDZ_REPL_MODE") == "true" ||
		(isTerminal() && hasREPLIndicators())
}

// isTerminal checks if stdin is a terminal
func isTerminal() bool {
	fileInfo, _ := os.Stdin.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// hasREPLIndicators checks for REPL-specific indicators
func hasREPLIndicators() bool {
	// Check if we have REPL-specific environment variables or process names
	return strings.Contains(strings.Join(os.Args, " "), "interactive")
}

// OfferInteractiveSelection presents a list of items and allows user to select one
func OfferInteractiveSelection(f *factory.Factory, items []InteractiveSelector, entityType string) (*InteractiveSelector, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no %s available to select", entityType)
	}

	// If only one item, auto-select
	if len(items) == 1 {
		fmt.Fprintf(f.IOStreams.Out, "\n✨ Auto-selecting the only %s: %s\n", entityType, items[0].Name)
		return &items[0], nil
	}

	// Always use the simple text-based selection for better consistency
	// The TUI version can be enabled later as an option
	return selectWithPrompt(f, items, entityType)
}

// selectWithPrompt uses simple text-based selection
func selectWithPrompt(f *factory.Factory, items []InteractiveSelector, entityType string) (*InteractiveSelector, error) {
	fmt.Fprintf(f.IOStreams.Out, "\n🎯 Select %s to add to context (or press Enter to skip):\n", entityType)
	fmt.Fprintf(f.IOStreams.Out, "────────────────────────────────────────────\n")

	// Display items with numbers
	for i, item := range items {
		desc := ""
		if item.Description != "" {
			desc = fmt.Sprintf(" - %s", item.Description)
		}
		// Truncate ID for display
		displayID := item.ID
		if len(displayID) > 8 {
			displayID = displayID[:8] + "..."
		}

		// Use consistent spacing for single and double digit numbers
		if i+1 < 10 {
			fmt.Fprintf(f.IOStreams.Out, "  %d. %s (%s)%s\n", i+1, item.Name, displayID, desc)
		} else {
			fmt.Fprintf(f.IOStreams.Out, " %d. %s (%s)%s\n", i+1, item.Name, displayID, desc)
		}
	}

	fmt.Fprintf(f.IOStreams.Out, "\nEnter number (1-%d) or press Enter to skip: ", len(items))

	// Read user input
	scanner := bufio.NewScanner(f.IOStreams.In)
	if !scanner.Scan() {
		return nil, fmt.Errorf("failed to read input")
	}

	input := strings.TrimSpace(scanner.Text())

	// If empty, user chose to skip
	if input == "" {
		fmt.Fprintf(f.IOStreams.Out, "💡 No %s selected. Context unchanged.\n", entityType)
		return nil, nil
	}

	// Parse selection
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(items) {
		fmt.Fprintf(f.IOStreams.Err, "❌ Invalid selection. Please enter a number between 1 and %d\n", len(items))
		return nil, fmt.Errorf("invalid selection")
	}

	selected := &items[choice-1]
	fmt.Fprintf(f.IOStreams.Out, "✅ Selected %s: %s\n", entityType, selected.Name)

	return selected, nil
}

// SetREPLContext sets context in REPL if available
func SetREPLContext(ctx context.Context, entityType, id, name string) error {
	// Store context in environment variables for the REPL to pick up
	// This is a bridge mechanism until we have proper context sharing
	switch strings.ToLower(entityType) {
	case "organization":
		os.Setenv("MDZ_CONTEXT_ORG_ID", id)
		os.Setenv("MDZ_CONTEXT_ORG_NAME", name)
		// Clear dependent contexts
		_ = os.Unsetenv("MDZ_CONTEXT_LEDGER_ID")
		_ = os.Unsetenv("MDZ_CONTEXT_LEDGER_NAME")
		_ = os.Unsetenv("MDZ_CONTEXT_PORTFOLIO_ID")
		_ = os.Unsetenv("MDZ_CONTEXT_PORTFOLIO_NAME")
		_ = os.Unsetenv("MDZ_CONTEXT_ACCOUNT_ID")
		_ = os.Unsetenv("MDZ_CONTEXT_ACCOUNT_NAME")

		// Also set a flag to indicate context was just updated
		os.Setenv("MDZ_CONTEXT_UPDATED", "true")
	case "ledger":
		os.Setenv("MDZ_CONTEXT_LEDGER_ID", id)
		os.Setenv("MDZ_CONTEXT_LEDGER_NAME", name)
		// Clear dependent contexts
		_ = os.Unsetenv("MDZ_CONTEXT_PORTFOLIO_ID")
		_ = os.Unsetenv("MDZ_CONTEXT_PORTFOLIO_NAME")
		_ = os.Unsetenv("MDZ_CONTEXT_ACCOUNT_ID")
		_ = os.Unsetenv("MDZ_CONTEXT_ACCOUNT_NAME")

		// Also set a flag to indicate context was just updated
		os.Setenv("MDZ_CONTEXT_UPDATED", "true")
	case "portfolio":
		os.Setenv("MDZ_CONTEXT_PORTFOLIO_ID", id)
		os.Setenv("MDZ_CONTEXT_PORTFOLIO_NAME", name)
		// Clear dependent contexts
		_ = os.Unsetenv("MDZ_CONTEXT_ACCOUNT_ID")
		_ = os.Unsetenv("MDZ_CONTEXT_ACCOUNT_NAME")

		// Also set a flag to indicate context was just updated
		os.Setenv("MDZ_CONTEXT_UPDATED", "true")
	case "account":
		os.Setenv("MDZ_CONTEXT_ACCOUNT_ID", id)
		os.Setenv("MDZ_CONTEXT_ACCOUNT_NAME", name)

		// Also set a flag to indicate context was just updated
		os.Setenv("MDZ_CONTEXT_UPDATED", "true")
	}

	return nil
}

// CreateInteractiveWrapper wraps a list command to add interactive selection
func CreateInteractiveWrapper(f *factory.Factory, entityType string, listFunc func() ([]InteractiveSelector, error)) error {
	// Execute the original list function
	items, err := listFunc()
	if err != nil {
		return err
	}

	// Only offer interactive selection in REPL mode or when explicitly requested
	if !IsInREPL() {
		return nil
	}

	// Offer interactive selection
	selected, err := OfferInteractiveSelection(f, items, entityType)
	if err != nil {
		return err
	}

	// If user selected something, set it in context
	if selected != nil {
		fmt.Fprintf(f.IOStreams.Out, "\n🔄 Setting %s context...\n", entityType)
		return SetREPLContext(context.Background(), entityType, selected.ID, selected.Name)
	}

	return nil
}
