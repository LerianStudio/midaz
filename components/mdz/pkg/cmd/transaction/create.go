// Package transaction provides commands for managing transactions in the Midaz CLI
package transaction

import (
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryTransactionCreate struct {
	factory        *factory.Factory
	repoTransaction repository.Transaction
	tuiInput       func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID    string
	LedgerID          string
	Type              string
	Description       string
	Status            string
	Amount            string
	Currency          string
	SourceAccountID   string
	DestinationAccountID string
	Metadata          string
	JSONFile          string
}

func (f *factoryTransactionCreate) runE(cmd *cobra.Command, _ []string) error {
	transaction := mmodel.CreateTransactionInput{}

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

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &transaction)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&transaction)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoTransaction.Create(f.OrganizationID, f.LedgerID, transaction)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Transaction", output.Created)

	return nil
}

func (f *factoryTransactionCreate) createRequestFromFlags(transaction *mmodel.CreateTransactionInput) error {
	var err error

	transaction.Type, err = utils.AssignStringField(f.Type, "type", f.tuiInput)
	if err != nil {
		return err
	}

	transaction.Description, err = utils.AssignStringField(f.Description, "description", f.tuiInput)
	if err != nil {
		return err
	}

	if len(f.Status) > 0 {
		transaction.Status = f.Status
	}

	if len(f.Amount) > 0 {
		transaction.Amount = f.Amount
	}

	if len(f.Currency) > 0 {
		transaction.Currency = f.Currency
	}

	transaction.SourceAccountID, err = utils.AssignStringField(f.SourceAccountID, "source-account-id", f.tuiInput)
	if err != nil {
		return err
	}

	transaction.DestinationAccountID, err = utils.AssignStringField(f.DestinationAccountID, "destination-account-id", f.tuiInput)
	if err != nil {
		return err
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	transaction.Metadata = metadata

	return nil
}

func (f *factoryTransactionCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.Type, "type", "", "Specify the transaction type (e.g., TRANSFER, DEPOSIT, WITHDRAWAL).")
	cmd.Flags().StringVar(&f.Description, "description", "", "Provide a description for the transaction.")
	cmd.Flags().StringVar(&f.Status, "status", "", "Specify the status of the transaction (e.g., PENDING, COMPLETED).")
	cmd.Flags().StringVar(&f.Amount, "amount", "", "Specify the transaction amount.")
	cmd.Flags().StringVar(&f.Currency, "currency", "", "Specify the currency code (e.g., USD).")
	cmd.Flags().StringVar(&f.SourceAccountID, "source-account-id", "", "Specify the source account ID.")
	cmd.Flags().StringVar(&f.DestinationAccountID, "destination-account-id", "", "Specify the destination account ID.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing transaction attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryTransactionCreate {
	return &factoryTransactionCreate{
		factory:        f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:       tui.Input,
	}
}

func newCmdTransactionCreate(f *factoryTransactionCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a transaction.",
		Long: utils.Format(
			"Creates a new transaction in the specified ledger. Allows for transfers,",
			"deposits, or withdrawals between accounts. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz transaction create",
			"$ mdz transaction create -h",
			"$ mdz transaction create --json-file payload.json",
			"$ cat payload.json | mdz transaction create --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}