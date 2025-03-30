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
	cmd.AddCommand(newCmdBalanceList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdBalanceDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdBalanceListByAccount(newInjectFacListByAccount(f.factory)))
	cmd.AddCommand(newCmdBalanceDelete(newInjectFacDelete(f.factory)))
}

func NewCmdBalance(f *factory.Factory) *cobra.Command {
	fBalance := factoryBalance{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Manages balances within a ledger.",
		Long: utils.Format(
			"The balance command allows you to list, describe, and delete balances",
			"within a ledger. Balances represent the current state of accounts",
			"after all transactions have been applied. Each action is carried out",
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
