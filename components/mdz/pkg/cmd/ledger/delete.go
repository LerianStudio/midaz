package ledger

import (
	"github.com/LerianStudio/midaz/v3/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/v3/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryLedgerDelete struct {
	factory        *factory.Factory
	repoLedger     repository.Ledger
	tuiInput       func(message string) (string, error)
	organizationID string
	ledgerID       string
}

func (f *factoryLedgerDelete) runE(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("organization-id") && len(f.organizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.organizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.ledgerID) < 1 {
		id, err := tui.Input("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.ledgerID = id
	}

	err := f.repoLedger.Delete(f.organizationID, f.ledgerID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.ledgerID, "Ledger", output.Deleted)

	return nil
}

func (f *factoryLedgerDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.organizationID, "organization-id", "",
		"Specify the organization ID")
	cmd.Flags().StringVar(&f.ledgerID, "ledger-id", "",
		"Specify the ledger ID to delete.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryLedgerDelete {
	return &factoryLedgerDelete{
		factory:    f,
		repoLedger: rest.NewLedger(f),
		tuiInput:   tui.Input,
	}
}

func newCmdLedgerDelete(f *factoryLedgerDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove a specific organization in Midaz",
		Long: "The /`organization delete/` command allows you to remove a specific organization in Midaz " +
			"by specifying the organization ID.",
		Example: utils.Format(
			"$ mdz organization delete --organization-id 12312",
			"$ mdz organization delete -i 12314",
			"$ mdz organization delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
