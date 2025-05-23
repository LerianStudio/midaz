package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInitialInputModel(t *testing.T) {
	message := "Enter your name:"
	model := initialInputModel(message)

	if model.message != message {
		t.Errorf("Expected message '%s', got '%s'", message, model.message)
	}

	if model.inputDone {
		t.Error("inputDone should be false initially")
	}

	if model.textInput.Placeholder != "..." {
		t.Errorf("Expected placeholder '...', got '%s'", model.textInput.Placeholder)
	}

	if model.textInput.CharLimit != 256 {
		t.Errorf("Expected CharLimit 256, got %d", model.textInput.CharLimit)
	}

	if model.textInput.Width != 100 {
		t.Errorf("Expected Width 100, got %d", model.textInput.Width)
	}
}

func TestInputModel_Init(t *testing.T) {
	model := initialInputModel("test")
	cmd := model.Init()

	if cmd == nil {
		t.Error("Init should return a command")
	}
}

func TestInputModel_View(t *testing.T) {
	message := "Enter value:"
	model := initialInputModel(message)

	view := model.View()

	if !strings.Contains(view, message) {
		t.Errorf("View should contain message '%s', got '%s'", message, view)
	}

	// Should contain the textinput view
	if len(view) < len(message) {
		t.Error("View should contain more than just the message")
	}
}

func TestInputModel_Update_KeyCtrlC(t *testing.T) {
	model := initialInputModel("test")

	// Simulate Ctrl+C key press
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updatedModel, cmd := model.Update(keyMsg)

	// Should quit on Ctrl+C
	if cmd == nil {
		t.Error("Update should return quit command on Ctrl+C")
	}

	// Model should remain the same type
	if _, ok := updatedModel.(inputModel); !ok {
		t.Error("Updated model should still be inputModel")
	}
}

func TestInputModel_Update_KeyEsc(t *testing.T) {
	model := initialInputModel("test")

	// Simulate Esc key press
	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	updatedModel, cmd := model.Update(keyMsg)

	// Should quit on Esc
	if cmd == nil {
		t.Error("Update should return quit command on Esc")
	}

	// Model should remain the same type
	if _, ok := updatedModel.(inputModel); !ok {
		t.Error("Updated model should still be inputModel")
	}
}

func TestInputModel_Update_KeyEnter(t *testing.T) {
	model := initialInputModel("test")

	// Simulate Enter key press
	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedModel, cmd := model.Update(keyMsg)

	// Should quit on Enter
	if cmd == nil {
		t.Error("Update should return quit command on Enter")
	}

	// inputDone should be set to true
	if updatedInputModel, ok := updatedModel.(inputModel); ok {
		if !updatedInputModel.inputDone {
			t.Error("inputDone should be true after Enter key")
		}
	} else {
		t.Error("Updated model should be inputModel")
	}
}

func TestInputModel_Update_RegularKey(t *testing.T) {
	model := initialInputModel("test")

	// Simulate regular character input
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updatedModel, cmd := model.Update(keyMsg)

	// Should not quit on regular key
	_ = cmd // We can't easily test the exact command type

	// Model should remain the same type
	if _, ok := updatedModel.(inputModel); !ok {
		t.Error("Updated model should still be inputModel")
	}
}

func TestFuzzySelectOption(t *testing.T) {
	option := FuzzySelectOption{
		ID:          "test-id",
		Name:        "Test Name",
		Description: "Test Description",
	}

	if option.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", option.ID)
	}
	if option.Name != "Test Name" {
		t.Errorf("Expected Name 'Test Name', got '%s'", option.Name)
	}
	if option.Description != "Test Description" {
		t.Errorf("Expected Description 'Test Description', got '%s'", option.Description)
	}
}

func TestInitialFuzzySelectModel(t *testing.T) {
	title := "Test Selection"
	options := []FuzzySelectOption{
		{ID: "1", Name: "Option 1", Description: "First option"},
		{ID: "2", Name: "Option 2", Description: "Second option"},
	}

	model := initialFuzzySelectModel(title, options)

	// Basic validation that model is created
	if model.cursor != 0 {
		t.Errorf("Expected cursor 0, got %d", model.cursor)
	}

	if model.selected != nil {
		t.Error("selected should be nil initially")
	}
}
