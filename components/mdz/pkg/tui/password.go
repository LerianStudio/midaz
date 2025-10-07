// Package tui provides terminal user interface components for the MDZ CLI.
// This file contains password input functionality with masked characters.
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Password prompts the user for password input with masked characters.
//
// This function creates a Bubble Tea password input interface with:
//   - Custom prompt message
//   - Masked input (displays * for each character)
//   - Character limit (50 chars)
//   - Ctrl+C/Esc to cancel
//   - Enter to submit
//
// Parameters:
//   - message: Prompt message to display
//
// Returns:
//   - string: User password input
//   - error: Error if program fails
func Password(message string) (string, error) {
	return runPasswordInput(initialPasswordInputModel(message))
}

// runPasswordInput runs the Bubble Tea program for password input.
//
// Parameters:
//   - m: Initial model (passwordModel)
//
// Returns:
//   - string: User password input
//   - error: Error if program fails
func runPasswordInput(m tea.Model) (string, error) {
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("error starting program: %w", err)
	}

	if im, ok := finalModel.(passwordModel); ok && im.inputDone {
		return im.textInput.Value(), nil
	}

	return "", nil
}

// passwordModel represents the Bubble Tea model for password input.
type passwordModel struct {
	textInput textinput.Model // Text input component with password masking
	message   string          // Prompt message
	inputDone bool            // Whether input is complete
}

// initialPasswordInputModel creates a new passwordModel with password masking.
//
// Parameters:
//   - message: Prompt message to display
//
// Returns:
//   - passwordModel: Initialized password input model
func initialPasswordInputModel(message string) passwordModel {
	ti := textinput.New()
	ti.Placeholder = "..."
	ti.Focus()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.CharLimit = 50
	ti.Width = 20

	return passwordModel{textInput: ti, message: message}
}

// Init initializes the password input model (Bubble Tea interface).
func (m passwordModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles input events and updates the model (Bubble Tea interface).
func (m passwordModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

// View renders the password input interface (Bubble Tea interface).
func (m passwordModel) View() string {
	return fmt.Sprintf("%s %s\n", m.message, m.textInput.View())
}
