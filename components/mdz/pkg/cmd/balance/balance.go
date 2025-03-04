// Package balance provides commands for managing balances in the Midaz CLI
package balance

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryBalance struct {
	factory *factory.Factory
}

func (f *factoryBalance) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdBalanceCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdBalanceList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdBalanceDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdBalanceUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdBalanceDelete(newInjectFacDelete(f.factory)))
}

// NewCmdBalance creates a new cobra command for managing balances
func NewCmdBalance(f *factory.Factory) *cobra.Command {
	fBalance := factoryBalance{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Manages balances in a ledger.",
		Long: utils.Format(
			"The balance command allows you to create, list, describe,",
			"update, and delete balances within a ledger. Each action is carried out",
			"using a specific subcommand.",
		),
		Example: utils.Format(
			"$ mdz balance",
			"$ mdz balance -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fBalance.setCmds(cmd)

	return cmd
}