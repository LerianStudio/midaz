// Package tui provides terminal user interface components for the MDZ CLI.
// This file contains selection menu functionality.
package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// Select prompts the user to select an option from a list.
//
// This function creates a Bubble Tea selection menu with:
//   - Custom prompt message
//   - List of options to choose from
//   - Arrow key navigation
//   - Ctrl+C/Esc to cancel
//   - Enter to select
//
// Parameters:
//   - message: Prompt message to display
//   - options: List of options to choose from
//
// Returns:
//   - string: Selected option
//   - error: Error if no option selected or program fails
func Select(message string, options []string) (string, error) {
	model := initialSelectModel(message, options)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	if m, ok := finalModel.(selectModel); ok && m.selected {
		return m.choices[m.cursor], nil
	}

	return "", errors.New("no option selected")
}

type selectModel struct {
	choices  []string // List of options
	cursor   int      // Current cursor position
	selected bool     // Flag indicating whether an option was selected
	message  string   // Custom message
}

// initialSelectModel creates a new selectModel with default configuration.
//
// Parameters:
//   - message: Prompt message to display
//   - choices: List of options
//
// Returns:
//   - selectModel: Initialized select model
func initialSelectModel(message string, choices []string) selectModel {
	return selectModel{
		choices: choices,
		cursor:  0,
		message: message,
	}
}

// Init initializes the select model (Bubble Tea interface).
func (m selectModel) Init() tea.Cmd {
	return nil
}

// Update handles input events and updates the model (Bubble Tea interface).
func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyUp, tea.KeyCtrlP:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown, tea.KeyCtrlN:
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			m.selected = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View renders the selection menu interface (Bubble Tea interface).
func (m selectModel) View() string {
	s := m.message + "\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">" // add a “>” cursor to the current option
		}

		s += cursor + " " + choice + "\n"
	}

	s += "\nUse the up/down arrows to move the cursor and press Enter to select."

	return s
}
