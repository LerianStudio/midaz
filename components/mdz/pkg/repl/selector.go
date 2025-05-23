package repl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
)

// EntityType represents the type of entity being selected
type EntityType string

const (
	EntityOrganization EntityType = "organization"
	EntityLedger       EntityType = "ledger"
	EntityPortfolio    EntityType = "portfolio"
	EntityAccount      EntityType = "account"
	EntityAsset        EntityType = "asset"
	EntitySegment      EntityType = "segment"
)

// Entity represents a selectable entity
type Entity struct {
	ID          string
	Name        string
	Description string
	Type        EntityType
}

// Selector provides interactive entity selection functionality
type Selector struct {
	factory *factory.Factory
}

// NewSelector creates a new selector
func NewSelector(f *factory.Factory) *Selector {
	return &Selector{factory: f}
}

// SelectEntity presents a list of entities and allows the user to select one
func (s *Selector) SelectEntity(entityType EntityType, entities []Entity) (*Entity, error) {
	if len(entities) == 0 {
		return nil, fmt.Errorf("no %s found", entityType)
	}

	// If only one entity, auto-select it
	if len(entities) == 1 {
		fmt.Fprintf(s.factory.IOStreams.Out, "Auto-selecting %s: %s (%s)\n",
			entityType, entities[0].Name, entities[0].ID)
		return &entities[0], nil
	}

	// Display entities
	fmt.Fprintf(s.factory.IOStreams.Out, "\nAvailable %ss:\n", entityType)

	for i, entity := range entities {
		desc := ""
		if entity.Description != "" {
			desc = fmt.Sprintf(" - %s", entity.Description)
		}

		fmt.Fprintf(s.factory.IOStreams.Out, "  %d. %s (%s)%s\n",
			i+1, entity.Name, entity.ID, desc)
	}

	// Prompt for selection
	for {
		fmt.Fprintf(s.factory.IOStreams.Out, "\nSelect %s (1-%d, or 'q' to quit): ",
			entityType, len(entities))

		var input string

		_, err := fmt.Fscanln(s.factory.IOStreams.In, &input)
		if err != nil {
			continue
		}

		input = strings.TrimSpace(input)

		// Check for quit
		if strings.ToLower(input) == "q" || strings.ToLower(input) == "quit" {
			return nil, fmt.Errorf("selection cancelled")
		}

		// Parse number
		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(entities) {
			fmt.Fprintf(s.factory.IOStreams.Err, "Invalid selection. Please enter a number between 1 and %d\n",
				len(entities))
			continue
		}

		selected := &entities[choice-1]
		fmt.Fprintf(s.factory.IOStreams.Out, "Selected %s: %s (%s)\n",
			entityType, selected.Name, selected.ID)

		return selected, nil
	}
}

// SelectWithTUI uses the enhanced fuzzy TUI select component for better UX
func (s *Selector) SelectWithTUI(entityType EntityType, entities []Entity) (*Entity, error) {
	if len(entities) == 0 {
		return nil, fmt.Errorf("no %s found", entityType)
	}

	// If only one entity, auto-select it
	if len(entities) == 1 {
		fmt.Fprintf(s.factory.IOStreams.Out, "Auto-selecting %s: %s\n",
			entityType, entities[0].Name)
		return &entities[0], nil
	}

	// Prepare options for fuzzy TUI
	options := make([]tui.FuzzySelectOption, len(entities))
	for i, entity := range entities {
		options[i] = tui.FuzzySelectOption{
			ID:          entity.ID,
			Name:        entity.Name,
			Description: entity.Description,
		}
	}

	// Use enhanced fuzzy TUI select
	prompt := fmt.Sprintf("Select %s", entityType)

	selected, err := tui.FuzzySelect(prompt, options)
	if err != nil {
		return nil, err
	}

	// Find the selected entity
	for _, entity := range entities {
		if entity.ID == selected.ID {
			return &entity, nil
		}
	}

	return nil, fmt.Errorf("selection not found")
}

// ConfirmAction asks for user confirmation
func (s *Selector) ConfirmAction(message string) bool {
	fmt.Fprintf(s.factory.IOStreams.Out, "%s (y/N): ", message)

	var response string

	_, err := fmt.Fscanln(s.factory.IOStreams.In, &response)
	if err != nil {
		return false
	}

	response = strings.ToLower(strings.TrimSpace(response))

	return response == "y" || response == "yes"
}
