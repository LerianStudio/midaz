package ledger

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/spf13/cobra"
)

type factoryLedger struct {
	factory *factory.Factory
}

func (f *factoryLedger) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdLedgerCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdLedgerList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdLedgerDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdLedgerUpdate(newInjectFacUpdate(f.factory)))
}

func NewCmdLedger(f *factory.Factory) *cobra.Command {
	fOrg := factoryLedger{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "Manages ledgers to organize transactions within an organization",
		Long: `The ledger command allows you to create and manage financial records 
           called ledgers, which store all the transactions and operations 
           of an organization. Each organization can have multiple ledgers, 
           allowing you to segment records as needed, for example, 
           by country or by project.`,
		Example: utils.Format(
			"$ mdz ledger",
			"$ mdz ledger -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
