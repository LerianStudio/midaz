package asset

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/spf13/cobra"
)

type factoryAsset struct {
	factory *factory.Factory
}

func (f *factoryAsset) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdAssetCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdAssetList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdAssetDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdAssetUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdAssetDelete(newInjectFacDelete(f.factory)))
}

func NewCmdAsset(f *factory.Factory) *cobra.Command {
	fAsset := factoryAsset{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "asset",
		Short: "Manages the assets allowed in the ledger.",
		Long: utils.Format(
			"It centralizes the management of assets allowed in the ledger",
			"such as currencies, commodities and goods. The asset command makes",
			"it easy to create, update, remove and consult assets, which can be",
			"used in accounts and operations in the portfolio. These assets",
			"represent balance and are essential for transactions and management",
			"in the onboarding flow.",
		),
		Example: utils.Format(
			"$ mdz asset",
			"$ mdz asset -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fAsset.setCmds(cmd)

	return cmd
}
