// Package tui provides terminal user interface components for the MDZ CLI.
//
// This package uses the Bubble Tea framework to create interactive terminal
// interfaces for user input, including text input, password input, and selection menus.
package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Input prompts the user for text input with an interactive interface.
//
// This function creates a Bubble Tea text input interface with:
//   - Custom prompt message
//   - Character limit (256 chars)
//   - Empty value validation
//   - Ctrl+C/Esc to cancel
//   - Enter to submit
//
// Parameters:
//   - message: Prompt message to display
//
// Returns:
//   - string: User input value
//   - error: Error if input is empty or program fails
func Input(message string) (string, error) {
	return runInput(initialInputModel(message))
}

// runInput runs the Bubble Tea program for text input.
//
// Parameters:
//   - m: Initial model (inputModel)
//
// Returns:
//   - string: User input value
//   - error: Error if program fails or input is empty
func runInput(m tea.Model) (string, error) {
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("starting program: %w", err)
	}

	if im, ok := finalModel.(inputModel); ok && im.inputDone {
		if len(im.textInput.Value()) < 1 {
			return "", errors.New("the field cannot be empty")
		}

		return im.textInput.Value(), nil
	}

	return "", nil
}

// inputModel represents the Bubble Tea model for text input.
type inputModel struct {
	textInput textinput.Model // Text input component
	message   string          // Prompt message
	inputDone bool            // Whether input is complete
}

// initialInputModel creates a new inputModel with default configuration.
//
// Parameters:
//   - message: Prompt message to display
//
// Returns:
//   - inputModel: Initialized input model
func initialInputModel(message string) inputModel {
	ti := textinput.New()
	ti.Placeholder = "..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 100

	return inputModel{textInput: ti, message: message}
}

// Init initializes the input model (Bubble Tea interface).
func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles input events and updates the model (Bubble Tea interface).
func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			m.inputDone = true
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)

	return m, cmd
}

// View renders the input interface (Bubble Tea interface).
func (m inputModel) View() string {
	return fmt.Sprintf("%s %s\n", m.message, m.textInput.View())
}
