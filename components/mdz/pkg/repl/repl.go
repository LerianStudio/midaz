package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// REPL represents the Read-Eval-Print Loop
type REPL struct {
	factory  *factory.Factory
	rootCmd  *cobra.Command
	rl       *readline.Instance
	history  []string
	exitChan chan bool
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
		WelcomeMsg:   "Welcome to MDZ Interactive Mode! Type 'help' for commands or 'exit' to quit.",
		ExitCommands: []string{"exit", "quit", "q"},
	}
}

// New creates a new REPL instance
func New(factory *factory.Factory, rootCmd *cobra.Command, config *Config) (*REPL, error) {
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
	if factory.IOStreams.In != os.Stdin {
		rlConfig.Stdin = factory.IOStreams.In
	}
	if factory.IOStreams.Out != os.Stdout {
		rlConfig.Stdout = factory.IOStreams.Out
	}
	if factory.IOStreams.Err != os.Stderr {
		rlConfig.Stderr = factory.IOStreams.Err
	}

	rl, err := readline.NewEx(rlConfig)
	if err != nil {
		return nil, err
	}

	return &REPL{
		factory:  factory,
		rootCmd:  rootCmd,
		rl:       rl,
		history:  make([]string, 0),
		exitChan: make(chan bool),
	}, nil
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
			for _, exitCmd := range config.ExitCommands {
				if line == exitCmd {
					return nil
				}
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
	}

	// Reset root command for fresh execution
	r.rootCmd.SetArgs(args)
	r.rootCmd.SetIn(r.factory.IOStreams.In)
	r.rootCmd.SetOut(r.factory.IOStreams.Out)
	r.rootCmd.SetErr(r.factory.IOStreams.Err)

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
	var items []readline.PrefixCompleterInterface

	// Add built-in REPL commands
	items = append(items,
		readline.PcItem("history"),
		readline.PcItem("clear"),
		readline.PcItem("pwd"),
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
