package account

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryAccount struct {
	factory *factory.Factory
}

func (f *factoryAccount) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdAccountCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdAccountList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdAccountDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdAccountUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdAccountDelete(newInjectFacDelete(f.factory)))
}

func NewCmdAccount(f *factory.Factory) *cobra.Command {
	fAccount := factoryAccount{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manages accounts associated with a portfolio.",
		Long: utils.Format(
			"The account command allows you to create, update, list, describe",
			"and delete accounts within a portfolio. Each action is carried out",
			"using a specific subcommand. If an account already exists, the",
			"create and update operations will return an error.",
		),
		Example: utils.Format(
			"$ mdz account",
			"$ mdz account -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fAccount.setCmds(cmd)

	return cmd
}
