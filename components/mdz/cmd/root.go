package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/LerianStudio/midaz/components/mdz/cmd/login"
	"github.com/LerianStudio/midaz/components/mdz/cmd/ui"
	"github.com/LerianStudio/midaz/components/mdz/cmd/version"

	"github.com/spf13/cobra"
)

// NewRootCommand is a func that use cmd commands
func NewRootCommand() *cobra.Command {
	var cfgFile string

	var debug bool

	cmd := &cobra.Command{
		Use:   "mdz",
		Short: "mdz is the CLI interface to use Midaz services",
	}

	// Global flags
	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.mdz.yaml)")
	cmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug mode")

	// Subcommands

	// cmd.AddCommand(auth.NewCommand()) // TODO
	// cmd.AddCommand(ledger.NewCommand()) // TODO
	cmd.AddCommand(login.NewCommand())
	cmd.AddCommand(ui.NewCommand())
	cmd.AddCommand(version.NewCommand())

	return cmd
}

// Execute is a func that open nem root command
func Execute() {
	cobra.EnableCommandSorting = false
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	if err := NewRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	defer cancel()
}
