package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// \1 performs an operation
func Input(message string) (string, error) {
	return runInput(initialInputModel(message))
}

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

type inputModel struct {
	textInput textinput.Model
	message   string
	inputDone bool
}

func initialInputModel(message string) inputModel {
	ti := textinput.New()

	ti.Placeholder = "..."
	ti.Focus()

	ti.CharLimit = 256

	ti.Width = 100

	return inputModel{textInput: ti, message: message}
}

// func (m inputModel) Init() tea.Cmd { performs an operation
func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

// func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { performs an operation
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

// func (m inputModel) View() string { performs an operation
func (m inputModel) View() string {
	return fmt.Sprintf("%s %s\n", m.message, m.textInput.View())
}
