package root

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/version"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type factoryRoot struct {
	factory *factory.Factory
}

func (f *factoryRoot) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(version.NewCmdVersion(f.factory))
}

func (f *factoryRoot) setFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
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
		SilenceErrors: true, // Silence errors, so the help message won't be shown on flag error
		SilenceUsage:  true, // Silence usage on error
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
