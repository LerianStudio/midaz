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

type factoryTransactionDescribe struct {
	factory        *factory.Factory
	repoTransaction repository.Transaction
	tuiInput       func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID string
	LedgerID       string
	TransactionID  string
}

func (f *factoryTransactionDescribe) runE(cmd *cobra.Command, _ []string) error {
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

	resp, err := f.repoTransaction.GetByID(f.OrganizationID, f.LedgerID, f.TransactionID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp, "", "")

	return nil
}

func (f *factoryTransactionDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.TransactionID, "transaction-id", "", "Specify the transaction ID.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryTransactionDescribe {
	return &factoryTransactionDescribe{
		factory:        f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:       tui.Input,
	}
}

func newCmdTransactionDescribe(f *factoryTransactionDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes a transaction.",
		Long: utils.Format(
			"Retrieves and displays detailed information about a specific",
			"transaction in the specified ledger. Returns all available",
			"transaction details in JSON format.",
		),
		Example: utils.Format(
			"$ mdz transaction describe",
			"$ mdz transaction describe -h",
			"$ mdz transaction describe --organization-id org_123 --ledger-id ldg_456 --transaction-id txn_789",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}