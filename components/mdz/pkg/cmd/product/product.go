package product

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/spf13/cobra"
)

type factoryPortfolio struct {
	factory *factory.Factory
}

func (f *factoryPortfolio) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdProductCreate(newInjectFacCreate(f.factory)))
}

func NewCmdPortfolio(f *factory.Factory) *cobra.Command {
	fOrg := factoryPortfolio{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "portfolio",
		Short: "Manages the creation and maintenance of products for customer clustering.",
		Long: utils.Format(
			"The product command allows you to create and manage products, which",
			"represent groupings of customers based on specific criteria. This",
			"functionality makes it easier to define clustering policies for ",
			"better customer management, allowing you to configure, update, view",
			"and delete products.",
		),
		Example: utils.Format(
			"$ mdz product",
			"$ mdz product -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
