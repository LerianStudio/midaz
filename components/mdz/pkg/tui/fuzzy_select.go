package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FuzzySelectOption represents an option in the fuzzy selector
type FuzzySelectOption struct {
	ID          string
	Name        string
	Description string
}

// FuzzySelect creates a fuzzy searchable selector
func FuzzySelect(message string, options []FuzzySelectOption) (*FuzzySelectOption, error) {
	model := initialFuzzySelectModel(message, options)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()

	if err != nil {
		return nil, err
	}

	if m, ok := finalModel.(fuzzySelectModel); ok && m.selected != nil {
		return m.selected, nil
	}

	return nil, fmt.Errorf("no option selected")
}

type fuzzySelectModel struct {
	message         string
	allOptions      []FuzzySelectOption
	filteredOptions []FuzzySelectOption
	cursor          int
	searchInput     string
	selected        *FuzzySelectOption
	searchMode      bool
}

func initialFuzzySelectModel(message string, options []FuzzySelectOption) fuzzySelectModel {
	return fuzzySelectModel{
		message:         message,
		allOptions:      options,
		filteredOptions: options,
		cursor:          0,
		searchMode:      false,
	}
}

func (m fuzzySelectModel) Init() tea.Cmd {
	return nil
}

func (m fuzzySelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, nil
}

func (m fuzzySelectModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyCtrlS:
		return m.toggleSearchMode(), nil
	case tea.KeyUp, tea.KeyCtrlP:
		return m.moveCursorUp(), nil
	case tea.KeyDown, tea.KeyCtrlN:
		return m.moveCursorDown(), nil
	case tea.KeyEnter:
		return m.selectItem()
	case tea.KeyBackspace:
		return m.handleBackspace(), nil
	default:
		return m.handleTextInput(msg), nil
	}
}

func (m fuzzySelectModel) toggleSearchMode() fuzzySelectModel {
	m.searchMode = !m.searchMode
	if !m.searchMode {
		m.searchInput = ""
		m.filteredOptions = m.allOptions
		m.cursor = 0
	}

	return m
}

func (m fuzzySelectModel) moveCursorUp() fuzzySelectModel {
	if !m.searchMode && m.cursor > 0 {
		m.cursor--
	}

	return m
}

func (m fuzzySelectModel) moveCursorDown() fuzzySelectModel {
	if !m.searchMode && m.cursor < len(m.filteredOptions)-1 {
		m.cursor++
	}

	return m
}

func (m fuzzySelectModel) selectItem() (tea.Model, tea.Cmd) {
	if !m.searchMode && len(m.filteredOptions) > 0 {
		m.selected = &m.filteredOptions[m.cursor]
		return m, tea.Quit
	}

	return m, nil
}

func (m fuzzySelectModel) handleBackspace() fuzzySelectModel {
	if m.searchMode && len(m.searchInput) > 0 {
		m.searchInput = m.searchInput[:len(m.searchInput)-1]
		m.filterOptions()
	}

	return m
}

func (m fuzzySelectModel) handleTextInput(msg tea.KeyMsg) fuzzySelectModel {
	if m.searchMode && msg.Type == tea.KeyRunes {
		m.searchInput += string(msg.Runes)
		m.filterOptions()
	}

	return m
}

func (m *fuzzySelectModel) filterOptions() {
	if m.searchInput == "" {
		m.filteredOptions = m.allOptions
		m.cursor = 0

		return
	}

	search := strings.ToLower(m.searchInput)
	m.filteredOptions = nil

	for _, option := range m.allOptions {
		name := strings.ToLower(option.Name)
		id := strings.ToLower(option.ID)
		desc := strings.ToLower(option.Description)

		if strings.Contains(name, search) ||
			strings.Contains(id, search) ||
			strings.Contains(desc, search) {
			m.filteredOptions = append(m.filteredOptions, option)
		}
	}

	m.cursor = 0
}

func (m fuzzySelectModel) View() string {
	var s strings.Builder

	// Header style
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	// Search style
	searchStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86"))

	// Selected item style
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Background(lipgloss.Color("240")).
		Bold(true)

	// Normal item style
	itemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	// Description style
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)

	s.WriteString(headerStyle.Render(m.message) + "\n\n")

	// Search box
	if m.searchMode {
		s.WriteString(searchStyle.Render("🔍 Search: ") + m.searchInput + "█\n\n")
	} else {
		s.WriteString(searchStyle.Render("🔍 Press Ctrl+S to search\n\n"))
	}

	// Options
	if len(m.filteredOptions) == 0 {
		s.WriteString("No options found\n")
	} else {
		for i, option := range m.filteredOptions {
			cursor := "  "
			style := itemStyle

			if m.cursor == i && !m.searchMode {
				cursor = "▶ "
				style = selectedStyle
			}

			name := fmt.Sprintf("%s (%s)", option.Name, option.ID[:8])
			s.WriteString(cursor + style.Render(name))

			if option.Description != "" {
				s.WriteString(" " + descStyle.Render("- "+option.Description))
			}

			s.WriteString("\n")
		}
	}

	// Instructions
	s.WriteString("\n")

	if m.searchMode {
		s.WriteString("Type to search • Ctrl+S to exit search • Esc to quit")
	} else {
		s.WriteString("↑/↓ to navigate • Enter to select • Ctrl+S to search • Esc to quit")
	}

	return s.String()
}
