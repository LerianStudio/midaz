package interactive

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/repl"
	"github.com/spf13/cobra"
)

type factoryInteractive struct {
	factory *factory.Factory
}

func NewCmdInteractive(f *factory.Factory) *cobra.Command {
	fInteractive := &factoryInteractive{
		factory: f,
	}

	cmd := &cobra.Command{
		Use:     "interactive",
		Aliases: []string{"i", "repl"},
		Short:   "Start MDZ in interactive mode",
		Long: `Start MDZ in interactive mode with command history and auto-completion.

In interactive mode, you can:
- Execute any MDZ command without typing 'mdz' prefix
- Use tab completion for commands and flags
- Access command history with up/down arrows
- Use built-in commands: history, clear, pwd
- Exit with: exit, quit, or Ctrl+D`,
		Example: `  mdz interactive
  mdz i
  
  # Inside interactive mode:
  mdz> organization list
  mdz> ledger create --name "My Ledger"
  mdz> account list --organization-id <org-id> --ledger-id <ledger-id>
  mdz> history
  mdz> exit`,
		RunE: fInteractive.runE,
	}

	return cmd
}

func (f *factoryInteractive) runE(cmd *cobra.Command, args []string) error {
	// Get the root command from parent
	rootCmd := cmd.Root()

	// Create a copy of the root command for the REPL
	// This ensures a clean state for each REPL session
	replCmd := &cobra.Command{
		Use:   rootCmd.Use,
		Short: rootCmd.Short,
		Long:  rootCmd.Long,
	}

	// Copy all commands except interactive to avoid recursion
	for _, c := range rootCmd.Commands() {
		if c.Name() != "interactive" && c.Name() != "i" && c.Name() != "repl" {
			replCmd.AddCommand(c)
		}
	}

	// Create REPL configuration
	config := repl.DefaultConfig()

	// Customize prompt with organization/ledger context if available
	// Note: Config is not available on factory, so using simple prompt

	// Create and run REPL
	r, err := repl.New(f.factory, replCmd, config)
	if err != nil {
		return fmt.Errorf("failed to create interactive mode: %w", err)
	}
	defer r.Close()

	// Print banner
	if !f.factory.NoColor {
		fmt.Fprintln(f.factory.IOStreams.Out, "\033[36m"+banner+"\033[0m")
	} else {
		fmt.Fprintln(f.factory.IOStreams.Out, banner)
	}

	// Run the REPL
	ctx := context.Background()

	return r.Run(ctx, config)
}

const banner = `╔╦╗╔╦╗╔═╗  ╦╔╗╔╔╦╗╔═╗╦═╗╔═╗╔═╗╔╦╗╦╦  ╦╔═╗
║║║ ║║╔═╝  ║║║║ ║ ║╣ ╠╦╝╠═╣║   ║ ║╚╗╔╝║╣ 
╩ ╩═╩╝╚═╝  ╩╝╚╝ ╩ ╚═╝╩╚═╩ ╩╚═╝ ╩ ╩ ╚╝ ╚═╝`
