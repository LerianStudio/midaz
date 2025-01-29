package product

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryProduct struct {
	factory *factory.Factory
}

func (f *factoryProduct) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdProductCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdProductList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdProductDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdProductUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdProductDelete(newInjectFacDelete(f.factory)))
}

func NewCmdProduct(f *factory.Factory) *cobra.Command {
	fOrg := factoryProduct{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Shows details of a specific cluster.",
		Long: utils.Format(
			"The describe subcommand provides a detailed view of a specific",
			"cluster, including its clustering policies and the rules applied.",
			"Ideal for detailed analysis and monitoring of the settings governing",
			"the grouping of customers within each cluster.",
		),
		Example: utils.Format(
			"$ mdz cluster",
			"$ mdz cluster -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
