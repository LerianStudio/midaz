package transaction

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryTransactionRevert struct {
	factory        *factory.Factory
	repoTransaction repository.Transaction
	tuiInput       func(message string) (string, error)
	flagsRevert
}

type flagsRevert struct {
	OrganizationID  string
	LedgerID        string
	TransactionID   string
}

func (f *factoryTransactionRevert) runE(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := f.tuiInput("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := f.tuiInput("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.LedgerID = id
	}

	if !cmd.Flags().Changed("transaction-id") && len(f.TransactionID) < 1 {
		id, err := f.tuiInput("Enter your transaction-id")
		if err != nil {
			return err
		}

		f.TransactionID = id
	}

	resp, err := f.repoTransaction.Revert(f.OrganizationID, f.LedgerID, f.TransactionID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Reversal Transaction", output.Created)

	return nil
}

func (f *factoryTransactionRevert) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.TransactionID, "transaction-id", "", "Specify the transaction ID to revert.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacRevert(f *factory.Factory) *factoryTransactionRevert {
	return &factoryTransactionRevert{
		factory:        f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:       tui.Input,
	}
}

func newCmdTransactionRevert(f *factoryTransactionRevert) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Reverts a transaction.",
		Long: utils.Format(
			"Reverts a specific transaction by creating a new transaction that",
			"reverses the effects of the original transaction. This is useful for",
			"correcting errors or canceling transactions. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz transaction revert",
			"$ mdz transaction revert -h",
			"$ mdz transaction revert --organization-id <org-id> --ledger-id <ledger-id> --transaction-id <tx-id>",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
