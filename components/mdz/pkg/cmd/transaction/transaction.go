package transaction

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryTransaction struct {
	factory *factory.Factory
}

func (f *factoryTransaction) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdTransactionCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdTransactionCreateDSL(newInjectFacCreateDSL(f.factory)))
	cmd.AddCommand(newCmdTransactionList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdTransactionListByParent(newInjectFacListByParent(f.factory)))
	cmd.AddCommand(newCmdTransactionDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdTransactionUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdTransactionDelete(newInjectFacDelete(f.factory)))
	cmd.AddCommand(newCmdTransactionCommit(newInjectFacCommit(f.factory)))
	cmd.AddCommand(newCmdTransactionRevert(newInjectFacRevert(f.factory)))
}

// NewCmdTransaction creates a new cobra command for managing transactions
func NewCmdTransaction(f *factory.Factory) *cobra.Command {
	fTransaction := factoryTransaction{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "transaction",
		Short: "Manages transactions in a ledger.",
		Long: utils.Format(
			"The transaction command allows you to create, update, list, describe,",
			"commit, revert, and delete transactions within a ledger. You can create",
			"transactions using JSON or DSL syntax. Each action is carried out",
			"using a specific subcommand.",
		),
		Example: utils.Format(
			"$ mdz transaction",
			"$ mdz transaction -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fTransaction.setCmds(cmd)

	return cmd
}