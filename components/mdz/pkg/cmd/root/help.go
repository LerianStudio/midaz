package root

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type helpEntry struct {
	Title string
	Body  string
}

// help displays help for the current command, including description, synopsis,
// available commands, subcommands, examples and flag options.
func (f *factoryRoot) help(command *cobra.Command, args []string) {
	if isRootCmd(command.Parent()) && len(args) >= 2 && args[1] != "--help" && args[1] != "-h" {
		nestedSuggestFunc(command, args[1])
		return
	}

	baseCommands, subcmdCommands := f.collectCommands(command)
	examples := f.collectExamples(command)
	helpEntries := f.buildHelpEntries(command, baseCommands, subcmdCommands, examples)

	f.outputHelp(helpEntries, command)
}

// collectCommands collects base commands and subcommands
func (f *factoryRoot) collectCommands(command *cobra.Command) ([]string, []string) {
	var (
		baseCommands   []string
		subcmdCommands []string
	)

	for _, c := range command.Commands() {
		if c.Short == "" || c.Hidden {
			continue
		}

		s := rpad(c.Name(), c.NamePadding()) + c.Short

		if c.Annotations["Category"] == "skip" {
			continue
		}

		if !isRootCmd(c.Parent()) {
			subcmdCommands = append(subcmdCommands, s)
		} else {
			baseCommands = append(baseCommands, s)
		}
	}

	return baseCommands, subcmdCommands
}

// collectExamples collects help examples
func (f *factoryRoot) collectExamples(command *cobra.Command) []string {
	var examples []string

	if len(command.Example) > 0 {
		examples = append(examples, command.Example)
	}

	return examples
}

// buildHelpEntries builds the help entries
func (f *factoryRoot) buildHelpEntries(command *cobra.Command, baseCommands, subcmdCommands, examples []string) []helpEntry {
	var helpEntries []helpEntry

	longText := command.Long
	if longText == "" {
		longText = command.Short
	}

	helpEntries = append(helpEntries, helpEntry{"", color.New(color.Bold).Sprint(f.factory.CLIVersion)})
	if longText != "" {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(color.Bold).Sprint("DESCRIPTION"),
			Body:  color.New(color.FgHiWhite).Sprint(longText),
		})
	}

	helpEntries = append(helpEntries, helpEntry{
		Title: color.New(color.Bold).Sprint("SYNOPSIS"),
		Body:  color.New(color.FgHiWhite).Sprint(command.UseLine()),
	})

	if len(examples) > 0 {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(color.Bold).Sprint("EXAMPLES"),
			Body:  color.New(color.FgYellow).Sprint(strings.Join(examples, "\n")),
		})
	}

	if len(baseCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(color.Bold).Sprint("AVAILABLE COMMANDS"),
			Body:  color.New(color.FgHiWhite).Sprint(strings.Join(baseCommands, "\n")),
		})
	}

	if len(subcmdCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(color.Bold).Sprint("AVAILABLE SUBCOMMANDS"),
			Body:  color.New(color.FgHiWhite).Sprint(strings.Join(subcmdCommands, "\n")),
		})
	}

	flagUsages := command.LocalFlags().FlagUsages()
	if flagUsages != "" {
		if isRootCmd(command) {
			helpEntries = append(helpEntries, helpEntry{
				Title: color.New(color.Bold).Sprint("GLOBAL OPTIONS"),
				Body:  color.New(color.FgHiWhite).Sprint(dedent(flagUsages)),
			})
		} else {
			helpEntries = append(helpEntries, helpEntry{
				Title: color.New(color.Bold).Sprint("LOCAL OPTIONS"),
				Body:  color.New(color.FgHiWhite).Sprint(dedent(flagUsages)),
			})
		}
	}

	inheritedFlagUsages := command.InheritedFlags().FlagUsages()
	if inheritedFlagUsages != "" {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(color.Bold).Sprint("GLOBAL OPTIONS"),
			Body:  color.New(color.FgHiWhite).Sprint(dedent(inheritedFlagUsages)),
		})
	}

	helpEntries = append(helpEntries, helpEntry{
		Title: color.New(color.Bold).Sprint("LEARN MORE"),
		Body:  color.New(color.FgHiWhite).Sprint("Use 'mdz <command> <subcommand> --help' for more information about a command"),
	})

	return helpEntries
}

// outputHelp shows help entries
func (f *factoryRoot) outputHelp(helpEntries []helpEntry, command *cobra.Command) {
	out := command.OutOrStdout()

	// Loop over the help entries and print them
	for _, e := range helpEntries {
		if e.Title != "" {
			fmt.Fprintln(out, e.Title)
			fmt.Fprintln(out, Indent(strings.Trim(e.Body, "\r\n"), "  "))
		} else {
			fmt.Fprintln(out, e.Body)
		}

		fmt.Fprintln(out)
	}
}

// nestedSuggestFunc suggests corrections when an invalid command is supplied.
// If “help” is the argument, it suggests “--help”. Otherwise, it calculates suggestions
// based on the minimum distance between the supplied command and the available commands.
func nestedSuggestFunc(command *cobra.Command, arg string) {
	command.Printf("unknown command %q for %q\n", arg, command.CommandPath())

	var candidates []string

	if arg == "help" {
		candidates = []string{"--help"}
	} else {
		if command.SuggestionsMinimumDistance <= 0 {
			command.SuggestionsMinimumDistance = 2
		}

		candidates = command.SuggestionsFor(arg)
	}

	if len(candidates) > 0 {
		command.Print("\nDid you mean this?\n")

		for _, c := range candidates {
			command.Printf("\t%s\n", c)
		}
	}

	command.Print("\n")
}

// isRootCmd checks if the command is the root command
// root if it doesn't have a parent command (HasParent returns false)
func isRootCmd(command *cobra.Command) bool {
	return command != nil && !command.HasParent()
}

// rpad adds spacing to the right of a string up to the size specified in padding.
func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds ", padding)
	return fmt.Sprintf(template, s)
}

// dedent removes the smallest pkg indentation from all lines in a string.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1

	for _, l := range lines {
		if len(l) == 0 {
			continue
		}

		indent := len(l) - len(strings.TrimLeft(l, " "))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}

	if minIndent <= 0 {
		return s
	}

	var buf bytes.Buffer
	for _, l := range lines {
		fmt.Fprintln(&buf, strings.TrimPrefix(l, strings.Repeat(" ", minIndent)))
	}

	return strings.TrimSuffix(buf.String(), "\n")
}

// Indent Adds an indentation level to all lines of a string
func Indent(s, indent string) string {
	lineRE := regexp.MustCompile(`(?m)^`)

	if len(strings.TrimSpace(s)) == 0 {
		return s
	}

	return lineRE.ReplaceAllLiteralString(s, indent)
}
