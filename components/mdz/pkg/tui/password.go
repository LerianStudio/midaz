package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// \1 performs an operation
func Password(message string) (string, error) {
	return runPasswordInput(initialPasswordInputModel(message))
}

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

type passwordModel struct {
	textInput textinput.Model
	message   string
	inputDone bool
}

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

// func (m passwordModel) Init() tea.Cmd { performs an operation
func (m passwordModel) Init() tea.Cmd {
	return textinput.Blink
}

// func (m passwordModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { performs an operation
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

// func (m passwordModel) View() string { performs an operation
func (m passwordModel) View() string {
	return fmt.Sprintf("%s %s\n", m.message, m.textInput.View())
}
