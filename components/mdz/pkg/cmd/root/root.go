// Package root provides the root command for the MDZ CLI.
//
// This package creates the main "mdz" command and registers all subcommands.
// It handles global flags, configuration loading, and command initialization.
package root

import (
	"errors"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/account"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/asset"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/configure"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/ledger"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/login"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/organization"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/portfolio"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/segment"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/version"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/setting"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// factoryRoot wraps the factory for the root command.
type factoryRoot struct {
	factory *factory.Factory
}

// setCmds registers all subcommands to the root command.
//
// This method adds all entity management commands:
//   - version: Display CLI version
//   - login: Authenticate with Midaz
//   - organization: Manage organizations
//   - ledger: Manage ledgers
//   - asset: Manage assets
//   - portfolio: Manage portfolios
//   - segment: Manage segments
//   - account: Manage accounts
//   - configure: Configure API endpoints
func (f *factoryRoot) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(version.NewCmdVersion(f.factory))
	cmd.AddCommand(login.NewCmdLogin(f.factory))
	cmd.AddCommand(organization.NewCmdOrganization(f.factory))
	cmd.AddCommand(ledger.NewCmdLedger(f.factory))
	cmd.AddCommand(asset.NewCmdAsset(f.factory))
	cmd.AddCommand(portfolio.NewCmdPortfolio(f.factory))
	cmd.AddCommand(segment.NewCmdSegment(f.factory))
	cmd.AddCommand(account.NewCmdAccount(f.factory))
	cmd.AddCommand(configure.NewCmdConfigure(configure.NewInjectFacConfigure(f.factory)))
}

// setFlags configures global flags for the root command.
//
// This method adds:
//   - --no-color: Disable colored output (persistent flag)
//   - -h, --help: Display help information
func (f *factoryRoot) setFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&f.factory.NoColor, "no-color", false, "Changes the output format passing the json value to the flag")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

// persistentPreRunE loads configuration before running any command.
//
// This hook:
// 1. Skips configuration loading for commands that don't need it (completion, version)
// 2. Reads saved configuration from ~/.config/mdz/mdz.toml
// 3. Loads token and API endpoints into factory
// 4. Returns error if configuration is missing (prompts user to login)
//
// Parameters:
//   - cmd: Cobra command being executed
//   - _: Command arguments (unused)
//
// Returns:
//   - error: Error if configuration cannot be loaded
func (f *factoryRoot) persistentPreRunE(cmd *cobra.Command, _ []string) error {
	name := cmd.Name()

	if name == "completion" ||
		name == "bash" ||
		name == "fish" ||
		name == "powershell" ||
		name == "zsh" ||
		name == "version" {
		return nil
	}

	sett, err := setting.Read()
	if err != nil {
		return errors.New("Try the login command first 'mdz login -h' " + err.Error())
	}

	if len(sett.ClientID) > 0 {
		f.factory.Env.ClientID = sett.ClientID
	}

	if len(sett.ClientSecret) > 0 {
		f.factory.Env.ClientSecret = sett.ClientSecret
	}

	if len(sett.URLAPIAuth) > 0 {
		f.factory.Env.URLAPIAuth = sett.URLAPIAuth
	}

	if len(sett.URLAPILedger) > 0 {
		f.factory.Env.URLAPILedger = sett.URLAPILedger
	}

	if len(sett.Token) > 0 {
		f.factory.Token = sett.Token
	}

	return nil
}

// NewCmdRoot creates the root "mdz" command with all subcommands.
//
// This function:
// 1. Creates factoryRoot wrapper
// 2. Configures root Cobra command with metadata
// 3. Sets up I/O streams
// 4. Configures help and version templates
// 5. Registers all subcommands
// 6. Sets up global flags
// 7. Configures persistent pre-run hook for config loading
//
// Parameters:
//   - f: Factory with dependencies
//
// Returns:
//   - *cobra.Command: Configured root command ready to execute
func NewCmdRoot(f *factory.Factory) *cobra.Command {
	fRoot := factoryRoot{
		factory: f,
	}

	cmd := &cobra.Command{
		Use:   "mdz",
		Short: "Midaz CLI is a command-line tool for interacting with the Midaz ledger",
		Long: utils.Format(
			"Midaz is an open-source ledger designed to offer multi-asset and multi-currency",
			"transaction capabilities within a single, natively immutable and fully auditable platform.",
		),
		Version: fRoot.factory.Env.Version,
		Example: utils.Format(
			"$ mdz",
			"$ mdz -h",
			"$ mdz --help",
		),
		PersistentPreRunE: fRoot.persistentPreRunE,
		SilenceErrors:     true, // Silence errors, so the help message won't be shown on flag error
		SilenceUsage:      true, // Silence usage on error
	}

	cmd.SetIn(fRoot.factory.IOStreams.In)
	cmd.SetOut(fRoot.factory.IOStreams.Out)
	cmd.SetErr(fRoot.factory.IOStreams.Err)

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fRoot.help(cmd, args)
	})

	cmd.SetVersionTemplate(color.New(color.Bold).Sprint(fRoot.factory.Env.Version))
	fRoot.setCmds(cmd)
	fRoot.setFlags(cmd)

	return cmd
}
