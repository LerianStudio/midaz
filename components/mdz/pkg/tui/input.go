package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func Input(message string) (string, error) {
	return runInput(initialInputModel(message))
}

func runInput(m tea.Model) (string, error) {
	p := tea.NewProgram(m)
	finalModel, err := p.Run()

	if err != nil {
		return "", fmt.Errorf("erro ao iniciar o programa: %w", err)
	}

	if pm, ok := finalModel.(inputModel); ok && pm.entered {
		return pm.input, nil
	}

	return "", nil
}

func initialInputModel(message string) inputModel {
	return inputModel{message: message}
}

type inputModel struct {
	message string
	input   string
	cursor  int
	entered bool
}

func (m inputModel) Init() tea.Cmd {
	return nil
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case ctrlC:
			return m, tea.Quit
		case enter:
			m.entered = true
			return m, tea.Quit
		case backspace:
			if m.cursor > 0 {
				m.input = m.input[:m.cursor-1] + m.input[m.cursor:]
				m.cursor--
			}
		case left:
			if m.cursor > 0 {
				m.cursor--
			}
		case right:
			if m.cursor < len(m.input) {
				m.cursor++
			}
		default:
			// Adiciona o caractere digitado à entrada
			m.input = m.input[:m.cursor] + msg.String() + m.input[m.cursor:]
			m.cursor++
		}
	}

	return m, nil
}

func (m inputModel) View() string {
	if m.entered {
		return "Entry received!\n"
	}

	return fmt.Sprintf("%s: %s\n", m.message, m.input)
}

// Example of using
// func main() {
// 	input, err := Input("Enter your name")
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
// 		os.Exit(1)
// 	}

// 	fmt.Printf("Name entered: %s\n", input)
