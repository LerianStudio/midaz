package operation

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryOperation struct {
	factory *factory.Factory
}

func (f *factoryOperation) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdOperationList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdOperationDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdOperationListByAccount(newInjectFacListByAccount(f.factory)))
}

func NewCmdOperation(f *factory.Factory) *cobra.Command {
	fOperation := factoryOperation{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "operation",
		Short: "Manages operations within a ledger.",
		Long: utils.Format(
			"The operation command allows you to list and describe operations",
			"within a ledger. Operations are the individual debit and credit entries",
			"that make up transactions. Each action is carried out using a specific subcommand.",
		),
		Example: utils.Format(
			"$ mdz operation",
			"$ mdz operation -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOperation.setCmds(cmd)

	return cmd
}
