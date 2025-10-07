// Package main is the entry point for the MDZ CLI application.
//
// MDZ (Midaz CLI) is a command-line interface for interacting with the Midaz ledger platform.
// It provides a user-friendly way to manage organizations, ledgers, accounts, assets, and
// other entities without writing code or using HTTP clients directly.
//
// The CLI is built using the Cobra framework and follows a factory pattern for dependency
// injection, making it testable and maintainable.
package main

import (
	"os"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/root"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/output"
)

// main is the entry point for the MDZ CLI application.
//
// This function:
// 1. Creates an environment instance (loads config, credentials)
// 2. Creates a factory for dependency injection
// 3. Creates the root Cobra command with all subcommands
// 4. Executes the command based on CLI arguments
// 5. Handles errors and exits with appropriate status code
//
// Exit Codes:
//   - 0: Success
//   - 1: Error (command execution failed or error printing failed)
func main() {
	env := environment.New()

	f := factory.NewFactory(env)
	cmd := root.NewCmdRoot(f)

	if err := cmd.Execute(); err != nil {
		printErr := output.Errorf(f.IOStreams.Err, err)
		if printErr != nil {
			output.Printf(os.Stderr, "Failed to print error output: "+printErr.Error())

			os.Exit(1)
		}

		os.Exit(1)
	}
}
