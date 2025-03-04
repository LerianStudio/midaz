package transaction

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryTransactionUpdate struct {
	factory        *factory.Factory
	repoTransaction repository.Transaction
	tuiInput       func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	TransactionID     string
	Description       string
	Status            string
	Metadata          string
	JSONFile          string
}

func (f *factoryTransactionUpdate) runE(cmd *cobra.Command, _ []string) error {
	transaction := mmodel.UpdateTransactionInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := f.tuiInput("Enter your organization-id")
		if err != nil {
			return errors.Wrap(err, "failed to get organization ID from input")
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := f.tuiInput("Enter your ledger-id")
		if err != nil {
			return errors.Wrap(err, "failed to get ledger ID from input")
		}

		f.LedgerID = id
	}

	if !cmd.Flags().Changed("transaction-id") && len(f.TransactionID) < 1 {
		id, err := f.tuiInput("Enter your transaction-id")
		if err != nil {
			return errors.Wrap(err, "failed to get transaction ID from input")
		}

		f.TransactionID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &transaction)
		if err != nil {
			return errors.UserError(err, "Verify if the file format is JSON or fix its content according to the JSON format specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.updateRequestFromFlags(&transaction)
		if err != nil {
			return errors.Wrap(err, "failed to update transaction request from flags")
		}
	}

	resp, err := f.repoTransaction.Update(f.OrganizationID, f.LedgerID, f.TransactionID, transaction)
	if err != nil {
		return errors.CommandError("transaction update", err)
	}

	output.FormatAndPrint(f.factory, resp.ID, "Transaction", output.Updated)

	return nil
}

func (f *factoryTransactionUpdate) updateRequestFromFlags(transaction *mmodel.UpdateTransactionInput) error {
	if len(f.Description) > 0 {
		transaction.Description = f.Description
	}

	if len(f.Status) > 0 {
		transaction.Status = f.Status
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.ValidationError("metadata", "Invalid JSON format for metadata")
	}

	transaction.Metadata = metadata

	return nil
}

func (f *factoryTransactionUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.TransactionID, "transaction-id", "", "Specify the transaction ID to update.")
	cmd.Flags().StringVar(&f.Description, "description", "", "Update the transaction description.")
	cmd.Flags().StringVar(&f.Status, "status", "", "Update the transaction status (e.g., PENDING, COMPLETED, FAILED).")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing transaction update attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryTransactionUpdate {
	return &factoryTransactionUpdate{
		factory:        f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:       tui.Input,
	}
}

func newCmdTransactionUpdate(f *factoryTransactionUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates a transaction.",
		Long: utils.Format(
			"Updates the details of an existing transaction in the specified ledger.",
			"Allows modifying the description, status, and metadata of a transaction.",
			"Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz transaction update",
			"$ mdz transaction update -h",
			"$ mdz transaction update --organization-id org_123 --ledger-id ldg_456 --transaction-id txn_789 --status COMPLETED",
			"$ mdz transaction update --json-file payload.json",
			"$ cat payload.json | mdz transaction update --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}