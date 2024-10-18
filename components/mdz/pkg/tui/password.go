package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func Password() (string, error) {
	return runPassword(initialPasswordModel())
}

func runPassword(m tea.Model) (string, error) {
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("error starting the program: %w", err)
	}

	if pm, ok := finalModel.(passwordModel); ok && pm.entered {
		return pm.password, nil
	}

	return "", nil
}

func initialPasswordModel() passwordModel {
	return passwordModel{}
}

type passwordModel struct {
	message  string
	password string
	cursor   int
	entered  bool
}

func (m passwordModel) Init() tea.Cmd {
	return nil
}

func (m passwordModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			m.entered = true
			return m, tea.Quit
		case "backspace":
			if m.cursor > 0 {
				m.password = m.password[:m.cursor-1] + m.password[m.cursor:]
				m.cursor--
			}
		case "left":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right":
			if m.cursor < len(m.password) {
				m.cursor++
			}
		default:
			// Adds the character entered to the password
			m.password = m.password[:m.cursor] + msg.String() + m.password[m.cursor:]
			m.cursor++
		}
	}
	return m, nil
}

func (m passwordModel) View() string {
	if m.entered {
		return "Password received!\n"
	}

	return fmt.Sprintf("Enter your password: %s\n", repeatAsterisks(len(m.password)))
}

// Função para repetir asteriscos
func repeatAsterisks(n int) string {
	return strings.Repeat("*", n)
}

// Example of using
// func main() {
// 	password, err := Password(InitialPasswordModel())
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
// 		os.Exit(1)
// 	}
//
// 	fmt.Printf("%s\n", password)
// }
