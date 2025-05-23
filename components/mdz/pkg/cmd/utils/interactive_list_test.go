package utils

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
)

// nopReadCloser wraps a Reader to implement ReadCloser
type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error { return nil }

func TestIsInREPL(t *testing.T) {
	// Clear environment first
	os.Unsetenv("MDZ_REPL_MODE")

	// Should return false when not in REPL mode
	if IsInREPL() {
		t.Error("IsInREPL should return false when MDZ_REPL_MODE is not set")
	}

	// Should return true when MDZ_REPL_MODE is set
	os.Setenv("MDZ_REPL_MODE", "true")
	defer os.Unsetenv("MDZ_REPL_MODE")

	if !IsInREPL() {
		t.Error("IsInREPL should return true when MDZ_REPL_MODE is set to true")
	}
}

func TestOfferInteractiveSelection_EmptyItems(t *testing.T) {
	f := createTestFactory()

	items := []InteractiveSelector{}

	result, err := OfferInteractiveSelection(f, items, "test")

	if err == nil {
		t.Error("Expected error for empty items")
	}
	if result != nil {
		t.Error("Expected nil result for empty items")
	}

	expectedError := "no test available to select"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestOfferInteractiveSelection_SingleItem(t *testing.T) {
	f := createTestFactory()

	items := []InteractiveSelector{
		{ID: "item-1", Name: "Test Item", Description: "Test Description", Type: "test"},
	}

	result, err := OfferInteractiveSelection(f, items, "test")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result for single item")
	}
	if result.ID != "item-1" {
		t.Errorf("Expected ID 'item-1', got '%s'", result.ID)
	}

	// Check output contains auto-selection message
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Auto-selecting") {
		t.Error("Output should contain auto-selection message")
	}
}

func TestOfferInteractiveSelection_UserSkips(t *testing.T) {
	f := createTestFactory()

	items := []InteractiveSelector{
		{ID: "item-1", Name: "Test Item", Description: "Test Description", Type: "test"},
		{ID: "item-2", Name: "Second Item", Description: "Second Description", Type: "test"},
	}

	// Simulate user pressing Enter (empty input)
	f.IOStreams.In = &nopReadCloser{strings.NewReader("\n")}

	result, err := OfferInteractiveSelection(f, items, "test")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result when user skips")
	}

	// Check output contains skip message
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "No test selected") {
		t.Error("Output should contain skip message")
	}
}

func TestSetREPLContext_Organization(t *testing.T) {
	// Clear environment before test
	clearEnvironment()
	defer clearEnvironment()

	err := SetREPLContext(context.Background(), "organization", "org-123", "Test Org")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check environment variables
	if os.Getenv("MDZ_CONTEXT_ORG_ID") != "org-123" {
		t.Error("Organization ID not set in environment")
	}
	if os.Getenv("MDZ_CONTEXT_ORG_NAME") != "Test Org" {
		t.Error("Organization name not set in environment")
	}

	// Check that dependent contexts are cleared
	if os.Getenv("MDZ_CONTEXT_LEDGER_ID") != "" {
		t.Error("Ledger context should be cleared when setting organization")
	}

	// Check update flag
	if os.Getenv("MDZ_CONTEXT_UPDATED") != "true" {
		t.Error("Context updated flag should be set")
	}
}

func TestSetREPLContext_Ledger(t *testing.T) {
	// Clear environment before test
	clearEnvironment()
	defer clearEnvironment()

	// Set organization first
	os.Setenv("MDZ_CONTEXT_ORG_ID", "org-123")

	err := SetREPLContext(context.Background(), "ledger", "ledger-456", "Test Ledger")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check environment variables
	if os.Getenv("MDZ_CONTEXT_LEDGER_ID") != "ledger-456" {
		t.Error("Ledger ID not set in environment")
	}
	if os.Getenv("MDZ_CONTEXT_LEDGER_NAME") != "Test Ledger" {
		t.Error("Ledger name not set in environment")
	}

	// Check that parent context is preserved
	if os.Getenv("MDZ_CONTEXT_ORG_ID") != "org-123" {
		t.Error("Organization context should be preserved")
	}

	// Check that dependent contexts are cleared
	if os.Getenv("MDZ_CONTEXT_PORTFOLIO_ID") != "" {
		t.Error("Portfolio context should be cleared when setting ledger")
	}
}

func TestCreateInteractiveWrapper(t *testing.T) {
	// Test in non-REPL mode
	os.Unsetenv("MDZ_REPL_MODE")

	f := createTestFactory()

	called := false
	listFunc := func() ([]InteractiveSelector, error) {
		called = true
		return []InteractiveSelector{
			{ID: "item-1", Name: "Test Item", Type: "test"},
		}, nil
	}

	err := CreateInteractiveWrapper(f, "test", listFunc)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("List function should be called")
	}

	// Should not show interactive selection in non-REPL mode
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if strings.Contains(output, "Select test") {
		t.Error("Should not show interactive selection in non-REPL mode")
	}
}

func TestIsTerminal(t *testing.T) {
	// Just test that the function doesn't panic
	result := isTerminal()
	// We can't test the actual value since it depends on the environment
	_ = result
}

func TestHasREPLIndicators(t *testing.T) {
	// Test that the function doesn't panic
	result := hasREPLIndicators()
	// We can't easily test the logic without modifying os.Args
	_ = result
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

func clearEnvironment() {
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
