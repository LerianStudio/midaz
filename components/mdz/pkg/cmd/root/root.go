package root

import (
	"errors"

	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/asset"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/ledger"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/login"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/organization"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/portfolio"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/version"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/setting"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type factoryRoot struct {
	factory *factory.Factory
}

func (f *factoryRoot) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(version.NewCmdVersion(f.factory))
	cmd.AddCommand(login.NewCmdLogin(f.factory))

	cmd.AddCommand(organization.NewCmdOrganization(f.factory))
	cmd.AddCommand(ledger.NewCmdLedger(f.factory))
	cmd.AddCommand(asset.NewCmdAsset(f.factory))
	cmd.AddCommand(portfolio.NewCmdPortfolio(f.factory))
}

func (f *factoryRoot) setFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func (f *factoryRoot) persistentPreRunE(cmd *cobra.Command, _ []string) error {
	if cmd.Name() != "login" &&
		cmd.Name() != "completion" &&
		cmd.Name() != "version" {
		sett, err := setting.Read()
		if err != nil {
			return errors.New("Try the login command first " + err.Error())
		}

		f.factory.Token = sett.Token
	}

	return nil
}

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
		Version: fRoot.factory.CLIVersion,
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

	cmd.SetVersionTemplate(color.New(color.Bold).Sprint(fRoot.factory.CLIVersion))
	fRoot.setCmds(cmd)
	fRoot.setFlags(cmd)

	return cmd
}
