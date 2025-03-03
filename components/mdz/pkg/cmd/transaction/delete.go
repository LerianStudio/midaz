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

type factoryTransactionDelete struct {
	factory        *factory.Factory
	repoTransaction repository.Transaction
	tuiInput       func(message string) (string, error)
	flagsDelete
}

type flagsDelete struct {
	OrganizationID string
	LedgerID       string
	TransactionID  string
}

func (f *factoryTransactionDelete) runE(cmd *cobra.Command, _ []string) error {
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

	err := f.repoTransaction.Delete(f.OrganizationID, f.LedgerID, f.TransactionID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.TransactionID, "Transaction", output.Deleted)

	return nil
}

func (f *factoryTransactionDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.TransactionID, "transaction-id", "", "Specify the transaction ID to delete.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryTransactionDelete {
	return &factoryTransactionDelete{
		factory:        f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:       tui.Input,
	}
}

func newCmdTransactionDelete(f *factoryTransactionDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes a transaction.",
		Long: utils.Format(
			"Permanently removes a transaction from the specified ledger.",
			"This action cannot be undone. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz transaction delete",
			"$ mdz transaction delete -h",
			"$ mdz transaction delete --organization-id org_123 --ledger-id ldg_456 --transaction-id txn_789",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}