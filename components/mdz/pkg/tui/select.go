package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// \1 performs an operation
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

func initialSelectModel(message string, choices []string) selectModel {
	return selectModel{
		choices: choices,
		cursor:  0,
		message: message,
	}
}

// func (m selectModel) Init() tea.Cmd { performs an operation
func (m selectModel) Init() tea.Cmd {
	return nil
}

// func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { performs an operation
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

// func (m selectModel) View() string { performs an operation
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
