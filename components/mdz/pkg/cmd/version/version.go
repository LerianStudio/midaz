package version

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type factoryVersion struct {
	factory *factory.Factory
}

func (f *factoryVersion) runE(_ *cobra.Command, _ []string) error {
	output.Printf(f.factory.IOStreams.Out, color.New(color.Bold).Sprint(f.factory.CLIVersion))
	return nil
}

func NewCmdVersion(f *factory.Factory) *cobra.Command {
	fVersion := factoryVersion{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Returns the CLI version",
		Long:  "Displays the version of the CLI binary installed in the computer.",
		Example: utils.Format(
			"$ mdz version",
			"$ mdz version -h",
		),
		RunE: fVersion.runE,
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")

	return cmd
}
