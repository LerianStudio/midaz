package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// REPL represents the Read-Eval-Print Loop
type REPL struct {
	factory     *factory.Factory
	rootCmd     *cobra.Command
	rl          *readline.Instance
	history     []string
	exitChan    chan bool
	context     *Context
	selector    *Selector
	interceptor *CommandInterceptor
}

// Config represents REPL configuration
type Config struct {
	Prompt       string
	HistoryFile  string
	MaxHistory   int
	WelcomeMsg   string
	ExitCommands []string
}

// DefaultConfig returns default REPL configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Prompt:       "mdz> ",
		HistoryFile:  homeDir + "/.mdz_history",
		MaxHistory:   1000,
		WelcomeMsg:   "Welcome to MDZ Interactive Mode!\n\nCommands:\n  help     - Show help for commands\n  context  - Show current context\n  use      - Set context (e.g., 'use organization <id>')\n  unset    - Clear context\n  exit     - Exit REPL\n\nThe REPL will automatically prompt for missing context when needed.",
		ExitCommands: []string{"exit", "quit", "q"},
	}
}

// New creates a new REPL instance
func New(f *factory.Factory, rootCmd *cobra.Command, config *Config) (*REPL, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create readline config
	rlConfig := &readline.Config{
		Prompt:            config.Prompt,
		HistoryFile:       config.HistoryFile,
		HistoryLimit:      config.MaxHistory,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		AutoComplete:      createCompleter(rootCmd),
	}

	// Set custom IO if factory has custom IOStreams
	if f.IOStreams.In != os.Stdin {
		rlConfig.Stdin = f.IOStreams.In
	}
	if f.IOStreams.Out != os.Stdout {
		rlConfig.Stdout = f.IOStreams.Out
	}
	if f.IOStreams.Err != os.Stderr {
		rlConfig.Stderr = f.IOStreams.Err
	}

	rl, err := readline.NewEx(rlConfig)
	if err != nil {
		return nil, err
	}

	repl := &REPL{
		factory:  f,
		rootCmd:  rootCmd,
		rl:       rl,
		history:  make([]string, 0),
		exitChan: make(chan bool),
		context:  NewContext(),
		selector: NewSelector(f),
	}

	// Create interceptor after REPL is created
	repl.interceptor = NewCommandInterceptor(repl, f)

	return repl, nil
}

// Run starts the REPL
func (r *REPL) Run(ctx context.Context, config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	// Print welcome message
	fmt.Fprintln(r.factory.IOStreams.Out, config.WelcomeMsg)
	fmt.Fprintln(r.factory.IOStreams.Out)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create a cancelable context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-sigChan:
			cancel()
			r.exitChan <- true
		case <-ctx.Done():
			r.exitChan <- true
		}
	}()

	// Main REPL loop
	for {
		select {
		case <-r.exitChan:
			return nil
		default:
			// Update prompt based on context
			r.rl.SetPrompt(r.context.GetPrompt())

			line, err := r.rl.Readline()
			if err != nil {
				if err == readline.ErrInterrupt {
					continue
				} else if err == io.EOF {
					return nil
				}
				return err
			}

			// Trim whitespace
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check for exit commands
			if slices.Contains(config.ExitCommands, line) {
				return nil
			}

			// Add to history
			r.history = append(r.history, line)

			// Execute command
			if err := r.executeCommand(ctx, line); err != nil {
				fmt.Fprintf(r.factory.IOStreams.Err, "Error: %v\n", err)
			}
		}
	}
}

// executeCommand executes a single command
func (r *REPL) executeCommand(ctx context.Context, input string) error {
	// Parse the input into arguments
	args := parseCommandLine(input)
	if len(args) == 0 {
		return nil
	}

	// Special handling for built-in REPL commands
	switch args[0] {
	case "history":
		return r.showHistory()
	case "clear":
		return r.clearScreen()
	case "pwd":
		pwd, _ := os.Getwd()
		fmt.Fprintln(r.factory.IOStreams.Out, pwd)
		return nil
	case "context":
		fmt.Fprintln(r.factory.IOStreams.Out, r.context.String())
		return nil
	case "use":
		if len(args) < 3 {
			fmt.Fprintln(r.factory.IOStreams.Err, "Usage: use <entity> <id>")
			fmt.Fprintln(r.factory.IOStreams.Err, "Example: use organization 123-456")
			return nil
		}
		return r.handleUseCommand(ctx, args[1], args[2])
	case "unset":
		if len(args) < 2 {
			r.context.Clear()
			fmt.Fprintln(r.factory.IOStreams.Out, "Cleared all context")
		} else {
			return r.handleUnsetCommand(args[1])
		}
		return nil
	}

	// Reset root command for fresh execution
	r.rootCmd.SetArgs(args)
	r.rootCmd.SetIn(r.factory.IOStreams.In)
	r.rootCmd.SetOut(r.factory.IOStreams.Out)
	r.rootCmd.SetErr(r.factory.IOStreams.Err)

	// Find the command that will be executed
	cmd, _, err := r.rootCmd.Find(args)
	if err != nil {
		return r.rootCmd.ExecuteContext(ctx)
	}

	// Intercept the command to provide context if needed
	if err := r.interceptor.InterceptCommand(ctx, cmd, args); err != nil {
		return err
	}

	// Execute the command
	return r.rootCmd.ExecuteContext(ctx)
}

// showHistory displays command history
func (r *REPL) showHistory() error {
	for i, cmd := range r.history {
		fmt.Fprintf(r.factory.IOStreams.Out, "%4d  %s\n", i+1, cmd)
	}
	return nil
}

// clearScreen clears the terminal screen
func (r *REPL) clearScreen() error {
	// ANSI escape code to clear screen
	fmt.Fprint(r.factory.IOStreams.Out, "\033[2J\033[H")
	return nil
}

// Close cleans up REPL resources
func (r *REPL) Close() error {
	return r.rl.Close()
}

// handleUseCommand handles the "use" command to set context
func (r *REPL) handleUseCommand(_ context.Context, entityType, id string) error {
	switch strings.ToLower(entityType) {
	case "organization", "org":
		// TODO: Fetch organization details to get the name
		r.context.SetOrganization(id, id)
		fmt.Fprintf(r.factory.IOStreams.Out, "Using organization: %s\n", id)
	case "ledger", "led":
		if r.context.OrganizationID == "" {
			fmt.Fprintln(r.factory.IOStreams.Err, "No organization selected. Use 'use organization <id>' first")
			return nil
		}
		// TODO: Fetch ledger details to get the name
		r.context.SetLedger(id, id)
		fmt.Fprintf(r.factory.IOStreams.Out, "Using ledger: %s\n", id)
	case "portfolio", "port":
		if r.context.LedgerID == "" {
			fmt.Fprintln(r.factory.IOStreams.Err, "No ledger selected. Use 'use ledger <id>' first")
			return nil
		}
		// TODO: Fetch portfolio details to get the name
		r.context.SetPortfolio(id, id)
		fmt.Fprintf(r.factory.IOStreams.Out, "Using portfolio: %s\n", id)
	case "account", "acc":
		if r.context.PortfolioID == "" {
			fmt.Fprintln(r.factory.IOStreams.Err, "No portfolio selected. Use 'use portfolio <id>' first")
			return nil
		}
		// TODO: Fetch account details to get the name
		r.context.SetAccount(id, id)
		fmt.Fprintf(r.factory.IOStreams.Out, "Using account: %s\n", id)
	default:
		fmt.Fprintf(r.factory.IOStreams.Err, "Unknown entity type: %s\n", entityType)
		fmt.Fprintln(r.factory.IOStreams.Err, "Valid types: organization, ledger, portfolio, account")
	}
	return nil
}

// handleUnsetCommand handles the "unset" command to clear context
func (r *REPL) handleUnsetCommand(entityType string) error {
	switch strings.ToLower(entityType) {
	case "organization", "org":
		r.context.Clear()
		fmt.Fprintln(r.factory.IOStreams.Out, "Cleared organization context")
	case "ledger", "led":
		r.context.ClearLedger()
		fmt.Fprintln(r.factory.IOStreams.Out, "Cleared ledger context")
	case "portfolio", "port":
		r.context.ClearPortfolio()
		fmt.Fprintln(r.factory.IOStreams.Out, "Cleared portfolio context")
	case "account", "acc":
		r.context.ClearAccount()
		fmt.Fprintln(r.factory.IOStreams.Out, "Cleared account context")
	default:
		fmt.Fprintf(r.factory.IOStreams.Err, "Unknown entity type: %s\n", entityType)
		fmt.Fprintln(r.factory.IOStreams.Err, "Valid types: organization, ledger, portfolio, account")
	}
	return nil
}

// GetContext returns the current REPL context
func (r *REPL) GetContext() *Context {
	return r.context
}

// parseCommandLine parses a command line into arguments
func parseCommandLine(input string) []string {
	var args []string
	var current strings.Builder
	var inQuote bool
	var quoteChar rune

	for i, char := range input {
		switch char {
		case '"', '\'':
			if !inQuote {
				inQuote = true
				quoteChar = char
			} else if char == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(char)
			}
		case ' ':
			if inQuote {
				current.WriteRune(char)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		case '\\':
			// Handle escape sequences
			if i+1 < len(input) {
				nextChar := input[i+1]
				switch nextChar {
				case 'n':
					current.WriteRune('\n')
				case 't':
					current.WriteRune('\t')
				case '\\':
					current.WriteRune('\\')
				case '"', '\'':
					current.WriteRune(rune(nextChar))
				default:
					current.WriteRune(char)
				}
			} else {
				current.WriteRune(char)
			}
		default:
			current.WriteRune(char)
		}
	}

	// Add the last argument
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// createCompleter creates an auto-completer for the REPL
func createCompleter(rootCmd *cobra.Command) *readline.PrefixCompleter {
	// Build completer from cobra commands
	// Preallocate with estimated size (5 built-in + cobra commands)
	items := make([]readline.PrefixCompleterInterface, 0, 5+len(rootCmd.Commands()))

	// Add built-in REPL commands
	items = append(items,
		readline.PcItem("history"),
		readline.PcItem("clear"),
		readline.PcItem("pwd"),
		readline.PcItem("context"),
		readline.PcItem("use",
			readline.PcItem("organization"),
			readline.PcItem("ledger"),
			readline.PcItem("portfolio"),
			readline.PcItem("account"),
		),
		readline.PcItem("unset",
			readline.PcItem("organization"),
			readline.PcItem("ledger"),
			readline.PcItem("portfolio"),
			readline.PcItem("account"),
		),
		readline.PcItem("exit"),
		readline.PcItem("quit"),
	)

	// Add all cobra commands
	for _, cmd := range rootCmd.Commands() {
		items = append(items, buildCommandCompleter(cmd))
	}

	return readline.NewPrefixCompleter(items...)
}

// buildCommandCompleter recursively builds completers for cobra commands
func buildCommandCompleter(cmd *cobra.Command) readline.PrefixCompleterInterface {
	var subItems []readline.PrefixCompleterInterface

	// Add subcommands
	for _, subCmd := range cmd.Commands() {
		if !subCmd.Hidden {
			subItems = append(subItems, buildCommandCompleter(subCmd))
		}
	}

	// Add flags
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Hidden {
			flagItem := "--" + flag.Name
			if flag.Shorthand != "" {
				subItems = append(subItems, readline.PcItem("-"+flag.Shorthand))
			}
			subItems = append(subItems, readline.PcItem(flagItem))
		}
	})

	// Add common flags
	subItems = append(subItems,
		readline.PcItem("-h"),
		readline.PcItem("--help"),
	)

	if len(subItems) > 0 {
		return readline.PcItem(cmd.Name(), subItems...)
	}
	return readline.PcItem(cmd.Name())
}
