package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

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

	return "", fmt.Errorf("no option selected")
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

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "ctrl+n":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectModel) View() string {
	s := fmt.Sprintf("%s\n\n", m.message)

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">" // add a “>” cursor to the current option
		}
		s += fmt.Sprintf("%s %s\n", cursor, choice)
	}

	s += "\nUse the up/down arrows to move the cursor and press Enter to select."

	return s
}

// Example of using
// func main() {
// 	answer, err := Select([]string{"Log in via browser", "Log in via terminal"})
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
// 		os.Exit(1)
// 	}
//
// 	fmt.Printf("You chose: %s\n", answer)
// }
