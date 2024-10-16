package root

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// help displays help for the current command, including description, synopsis,
// available commands, subcommands, examples and flag options.
func (f *factoryRoot) help(command *cobra.Command, args []string) {
	if isRootCmd(command.Parent()) && len(args) >= 2 && args[1] !=
		"--help" && args[1] != "-h" {
		nestedSuggestFunc(command, args[1])
		return
	}

	var (
		baseCommands   []string
		subcmdCommands []string
		examples       []string
	)

	// If the command is not a root command, add it to the list of subcommands.
	// Otherwise, add it to the list of base commands.
	for _, c := range command.Commands() {
		if c.Short == "" {
			continue
		}

		if c.Hidden {
			continue
		}

		s := rpad(c.Name(), c.NamePadding()) + c.Short

		switch {
		case c.Annotations["Category"] == "skip":
			continue
		case !isRootCmd(c.Parent()):
			// Help of subcommand
			subcmdCommands = append(subcmdCommands, s)
			continue
		default:
			baseCommands = append(baseCommands, s)
			continue
		}
	}

	type helpEntry struct {
		Title string
		Body  string
	}

	if len(command.Example) > 0 {
		examples = append(examples, command.Example)
	}

	longText := command.Long
	if longText == "" {
		longText = command.Short
	}

	styleTitle := color.Bold
	styleBody := color.FgHiWhite

	helpEntries := []helpEntry{}

	helpEntries = append(helpEntries, helpEntry{"", color.New(color.Bold).Sprint(f.factory.CLIVersion)})

	if longText != "" {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(styleTitle).Sprint("DESCRIPTION"),
			Body:  color.New(styleBody).Sprint(longText),
		})
	}

	helpEntries = append(helpEntries, helpEntry{
		Title: color.New(styleTitle).Sprint("SYNOPSIS"),
		Body:  color.New(styleBody).Sprint(command.UseLine()),
	})

	if len(examples) > 0 {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(styleTitle).Sprint("EXAMPLES"),
			Body:  color.New(color.FgYellow).Sprint(strings.Join(examples, "\n")),
		})
	}

	if len(baseCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(styleTitle).Sprint("AVAILABLE COMMANDS"),
			Body:  color.New(styleBody).Sprint(strings.Join(baseCommands, "\n")),
		})
	}

	if len(subcmdCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(styleTitle).Sprint("AVAILABLE SUBCOMMANDS"),
			Body:  color.New(styleBody).Sprint(strings.Join(subcmdCommands, "\n")),
		})
	}

	flagUsages := command.LocalFlags().FlagUsages()
	if flagUsages != "" {
		if isRootCmd(command) {
			helpEntries = append(helpEntries, helpEntry{
				Title: color.New(styleTitle).Sprint("GLOBAL OPTIONS"),
				Body:  color.New(styleBody).Sprint(dedent(flagUsages)),
			})
		} else {
			helpEntries = append(helpEntries, helpEntry{
				Title: color.New(styleTitle).Sprint("LOCAL OPTIONS"),
				Body:  color.New(styleBody).Sprint(dedent(flagUsages)),
			})
		}
	}

	inheritedFlagUsages := command.InheritedFlags().FlagUsages()
	if inheritedFlagUsages != "" {
		helpEntries = append(helpEntries, helpEntry{
			Title: color.New(styleTitle).Sprint("GLOBAL OPTIONS"),
			Body:  color.New(styleBody).Sprint(dedent(inheritedFlagUsages)),
		})
	}

	helpEntries = append(helpEntries, helpEntry{
		Title: color.New(styleTitle).Sprint("LEARN MORE"),
		Body:  color.New(styleBody).Sprint("Use 'mdz <command> <subcommand> --help' for more information about a command"),
	})

	out := command.OutOrStdout()
	for _, e := range helpEntries {
		if e.Title != "" {
			// If there is a title, add indentation to each line in the body
			fmt.Fprintln(out, e.Title)
			fmt.Fprintln(out, Indent(strings.Trim(e.Body, "\r\n"), "  "))
		} else {
			// If there is no title print the body as is
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

// dedent removes the smallest common indentation from all lines in a string.
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
	var lineRE = regexp.MustCompile(`(?m)^`)
	if len(strings.TrimSpace(s)) == 0 {
		return s
	}
	return lineRE.ReplaceAllLiteralString(s, indent)
}
