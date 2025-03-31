package assetrate

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryAssetRate struct {
	factory *factory.Factory
}

func (f *factoryAssetRate) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdAssetRateCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdAssetRateUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdAssetRateList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdAssetRateDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdAssetRateListByAsset(newInjectFacListByAsset(f.factory)))
}

func NewCmdAssetRate(f *factory.Factory) *cobra.Command {
	fAssetRate := factoryAssetRate{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "asset-rate",
		Short: "Manages asset rates within a ledger.",
		Long: utils.Format(
			"The asset-rate command allows you to create, update, list, and describe",
			"asset rates within a ledger. Asset rates define the exchange rates between",
			"different assets. Each action is carried out using a specific subcommand.",
		),
		Example: utils.Format(
			"$ mdz asset-rate",
			"$ mdz asset-rate -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fAssetRate.setCmds(cmd)

	return cmd
}
