// Package assetrate provides commands for managing asset rates in the Midaz CLI
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
	cmd.AddCommand(newCmdAssetRateList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdAssetRateDescribe(newInjectFacDescribe(f.factory)))
}

// NewCmdAssetRate creates a new cobra command for managing asset rates
func NewCmdAssetRate(f *factory.Factory) *cobra.Command {
	fAssetRate := factoryAssetRate{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "assetrate",
		Short: "Manages asset rates in a ledger.",
		Long: utils.Format(
			"The assetrate command allows you to create, list, and describe",
			"asset rates within a ledger. Each action is carried out",
			"using a specific subcommand.",
		),
		Example: utils.Format(
			"$ mdz assetrate",
			"$ mdz assetrate -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fAssetRate.setCmds(cmd)

	return cmd
}
