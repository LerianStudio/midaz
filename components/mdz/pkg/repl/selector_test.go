package repl

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestNewSelector(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	if selector == nil {
		t.Fatal("NewSelector should not return nil")
	}
	if selector.factory != f {
		t.Error("Selector should store factory reference")
	}
}

func TestSelector_SelectEntity_NoEntities(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	entities := []Entity{}

	_, err := selector.SelectEntity(EntityOrganization, entities)
	if err == nil {
		t.Error("Should return error when no entities provided")
	}
	if !strings.Contains(err.Error(), "no organization found") {
		t.Errorf("Error should mention no organization found, got: %v", err)
	}
}

func TestSelector_SelectEntity_SingleEntity(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	entities := []Entity{
		{
			ID:          "org-123",
			Name:        "Test Organization",
			Description: "Test Description",
			Type:        EntityOrganization,
		},
	}

	selected, err := selector.SelectEntity(EntityOrganization, entities)
	if err != nil {
		t.Errorf("Should not error with single entity: %v", err)
	}
	if selected == nil {
		t.Fatal("Selected entity should not be nil")
	}
	if selected.ID != "org-123" {
		t.Errorf("Expected selected ID 'org-123', got '%s'", selected.ID)
	}

	// Check output message
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Auto-selecting organization") {
		t.Error("Should show auto-selection message")
	}
}

func TestSelector_SelectEntity_MultipleEntities_Display(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	entities := []Entity{
		{
			ID:          "org-123",
			Name:        "Organization One",
			Description: "First org",
			Type:        EntityOrganization,
		},
		{
			ID:          "org-456",
			Name:        "Organization Two",
			Description: "",
			Type:        EntityOrganization,
		},
	}

	// Simulate quit input
	input := "q\n"
	f.IOStreams.In = &nopReadCloser{strings.NewReader(input)}

	_, err := selector.SelectEntity(EntityOrganization, entities)
	if err == nil || !strings.Contains(err.Error(), "selection cancelled") {
		t.Error("Should return cancellation error when user quits")
	}

	// Check output formatting
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	expectedElements := []string{
		"Available organizations:",
		"1. Organization One (org-123) - First org",
		"2. Organization Two (org-456)",
		"Select organization (1-2, or 'q' to quit):",
	}

	for _, element := range expectedElements {
		if !strings.Contains(output, element) {
			t.Errorf("Output should contain '%s', got: %s", element, output)
		}
	}
}

func TestSelector_SelectEntity_ValidSelection(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	entities := []Entity{
		{
			ID:   "org-123",
			Name: "Organization One",
			Type: EntityOrganization,
		},
		{
			ID:   "org-456",
			Name: "Organization Two",
			Type: EntityOrganization,
		},
	}

	// Simulate selecting first option
	input := "1\n"
	f.IOStreams.In = &nopReadCloser{strings.NewReader(input)}

	selected, err := selector.SelectEntity(EntityOrganization, entities)
	if err != nil {
		t.Errorf("Should not error with valid selection: %v", err)
	}
	if selected == nil {
		t.Fatal("Selected entity should not be nil")
	}
	if selected.ID != "org-123" {
		t.Errorf("Expected selected ID 'org-123', got '%s'", selected.ID)
	}

	// Check selection confirmation
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Selected organization: Organization One (org-123)") {
		t.Error("Should show selection confirmation")
	}
}

func TestSelector_SelectEntity_InvalidSelections(t *testing.T) {
	entities := []Entity{
		{ID: "org-123", Name: "Org One", Type: EntityOrganization},
		{ID: "org-456", Name: "Org Two", Type: EntityOrganization},
	}

	testCases := []struct {
		name     string
		input    string
		errorMsg string
	}{
		{
			name:     "Invalid number",
			input:    "abc\n1\n",
			errorMsg: "Invalid selection",
		},
		{
			name:     "Out of range - too high",
			input:    "5\n1\n",
			errorMsg: "Invalid selection",
		},
		{
			name:     "Out of range - zero",
			input:    "0\n1\n",
			errorMsg: "Invalid selection",
		},
		{
			name:     "Negative number",
			input:    "-1\n1\n",
			errorMsg: "Invalid selection",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := createTestFactory()
			selector := NewSelector(f)

			f.IOStreams.In = &nopReadCloser{strings.NewReader(tc.input)}

			selected, err := selector.SelectEntity(EntityOrganization, entities)
			if err != nil {
				t.Errorf("Should eventually succeed with valid input: %v", err)
			}
			if selected.ID != "org-123" {
				t.Errorf("Should select first entity after invalid attempts")
			}

			// Check error output
			errorOutput := f.IOStreams.Err.(*bytes.Buffer).String()
			if !strings.Contains(errorOutput, tc.errorMsg) {
				t.Errorf("Error output should contain '%s', got: %s", tc.errorMsg, errorOutput)
			}
		})
	}
}

func TestSelector_SelectEntity_QuitCommands(t *testing.T) {
	f := createTestFactory()
	_ = NewSelector(f)

	entities := []Entity{
		{ID: "org-123", Name: "Org One", Type: EntityOrganization},
	}

	quitCommands := []string{"q", "Q", "quit", "QUIT", "Quit"}

	for _, quitCmd := range quitCommands {
		t.Run(fmt.Sprintf("quit_%s", quitCmd), func(t *testing.T) {
			f := createTestFactory()
			selector := NewSelector(f)

			input := quitCmd + "\n"
			f.IOStreams.In = &nopReadCloser{strings.NewReader(input)}

			_, err := selector.SelectEntity(EntityOrganization, entities)
			if err == nil || !strings.Contains(err.Error(), "selection cancelled") {
				t.Errorf("Should return cancellation error for quit command '%s'", quitCmd)
			}
		})
	}
}

func TestSelector_SelectWithTUI_NoEntities(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	entities := []Entity{}

	_, err := selector.SelectWithTUI(EntityLedger, entities)
	if err == nil {
		t.Error("Should return error when no entities provided")
	}
	if !strings.Contains(err.Error(), "no ledger found") {
		t.Errorf("Error should mention no ledger found, got: %v", err)
	}
}

func TestSelector_SelectWithTUI_SingleEntity(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	entities := []Entity{
		{
			ID:          "ledger-456",
			Name:        "Test Ledger",
			Description: "Test Description",
			Type:        EntityLedger,
		},
	}

	selected, err := selector.SelectWithTUI(EntityLedger, entities)
	if err != nil {
		t.Errorf("Should not error with single entity: %v", err)
	}
	if selected == nil {
		t.Fatal("Selected entity should not be nil")
	}
	if selected.ID != "ledger-456" {
		t.Errorf("Expected selected ID 'ledger-456', got '%s'", selected.ID)
	}

	// Check output message
	output := f.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Auto-selecting ledger") {
		t.Error("Should show auto-selection message")
	}
}

func TestSelector_ConfirmAction_Yes(t *testing.T) {
	f := createTestFactory()
	_ = NewSelector(f)

	yesInputs := []string{"y", "Y", "yes", "YES", "Yes"}

	for _, input := range yesInputs {
		t.Run(fmt.Sprintf("yes_%s", input), func(t *testing.T) {
			f := createTestFactory()
			selector := NewSelector(f)

			f.IOStreams.In = &nopReadCloser{strings.NewReader(input + "\n")}

			result := selector.ConfirmAction("Continue?")
			if !result {
				t.Errorf("Should return true for '%s' input", input)
			}

			// Check prompt
			output := f.IOStreams.Out.(*bytes.Buffer).String()
			if !strings.Contains(output, "Continue? (y/N):") {
				t.Error("Should show confirmation prompt")
			}
		})
	}
}

func TestSelector_ConfirmAction_No(t *testing.T) {
	f := createTestFactory()
	_ = NewSelector(f)

	noInputs := []string{"n", "N", "no", "NO", "No", "", "anything"}

	for _, input := range noInputs {
		t.Run(fmt.Sprintf("no_%s", input), func(t *testing.T) {
			f := createTestFactory()
			selector := NewSelector(f)

			f.IOStreams.In = &nopReadCloser{strings.NewReader(input + "\n")}

			result := selector.ConfirmAction("Continue?")
			if result {
				t.Errorf("Should return false for '%s' input", input)
			}
		})
	}
}

func TestSelector_ConfirmAction_ReadError(t *testing.T) {
	f := createTestFactory()
	selector := NewSelector(f)

	// Use empty reader to cause read error
	f.IOStreams.In = &nopReadCloser{strings.NewReader("")}

	result := selector.ConfirmAction("Continue?")
	if result {
		t.Error("Should return false on read error")
	}
}

func TestEntityType_Constants(t *testing.T) {
	// Test that all entity type constants are properly defined
	expectedTypes := map[EntityType]string{
		EntityOrganization: "organization",
		EntityLedger:       "ledger",
		EntityPortfolio:    "portfolio",
		EntityAccount:      "account",
		EntityAsset:        "asset",
		EntitySegment:      "segment",
	}

	for entityType, expectedValue := range expectedTypes {
		if string(entityType) != expectedValue {
			t.Errorf("EntityType %v should equal '%s', got '%s'", entityType, expectedValue, string(entityType))
		}
	}
}

func TestEntity_Structure(t *testing.T) {
	entity := Entity{
		ID:          "test-id-123",
		Name:        "Test Entity",
		Description: "Test Description",
		Type:        EntityOrganization,
	}

	if entity.ID != "test-id-123" {
		t.Errorf("Expected ID 'test-id-123', got '%s'", entity.ID)
	}
	if entity.Name != "Test Entity" {
		t.Errorf("Expected Name 'Test Entity', got '%s'", entity.Name)
	}
	if entity.Description != "Test Description" {
		t.Errorf("Expected Description 'Test Description', got '%s'", entity.Description)
	}
	if entity.Type != EntityOrganization {
		t.Errorf("Expected Type EntityOrganization, got %v", entity.Type)
	}
}
