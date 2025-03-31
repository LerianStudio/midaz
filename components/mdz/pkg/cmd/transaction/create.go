package transaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

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
	factory         *factory.Factory
	repoTransaction repository.Transaction
	tuiInput        func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID       string
	LedgerID             string
	Description          string
	Template             string
	Amount               string
	AmountScale          string
	AssetCode            string
	ChartOfAccountsGroup string
	Source               string
	Destination          string
	ParentTransactionID  string
	StatusCode           string
	StatusDescription    string
	Metadata             string
	JSONFile             string
}

func (f *factoryTransactionCreate) runE(cmd *cobra.Command, _ []string) error {
	var err error

	if len(f.JSONFile) > 0 {
		// Read JSON file
		jsonFile, err := os.Open(f.JSONFile)
		if err != nil {
			return fmt.Errorf("opening JSON file: %v", err)
		}
		defer jsonFile.Close()

		byteValue, err := io.ReadAll(jsonFile)
		if err != nil {
			return fmt.Errorf("reading JSON file: %v", err)
		}

		var transaction mmodel.CreateTransactionInput
		if err := json.Unmarshal(byteValue, &transaction); err != nil {
			return fmt.Errorf("unmarshalling JSON: %v", err)
		}

		// Create transaction
		resp, err := f.repoTransaction.Create(f.OrganizationID, f.LedgerID, transaction)
		if err != nil {
			return err
		}

		output.FormatAndPrint(f.factory, resp.ID, "transaction", output.Created)

		return nil
	}

	transaction := mmodel.CreateTransactionInput{}

	if err := f.createRequestFromFlags(&transaction); err != nil {
		return err
	}

	resp, err := f.repoTransaction.Create(f.OrganizationID, f.LedgerID, transaction)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "transaction", output.Created)

	return nil
}

// processBasicFields handles the basic string fields and numeric conversions
func (f *factoryTransactionCreate) processBasicFields(transaction *mmodel.CreateTransactionInput) error {
	var err error

	transaction.Description, err = utils.AssignStringField(f.Description, "description", f.tuiInput)
	if err != nil {
		return err
	}

	transaction.Template, err = utils.AssignStringField(f.Template, "template", f.tuiInput)
	if err != nil {
		return err
	}

	transaction.AssetCode, err = utils.AssignStringField(f.AssetCode, "asset code", f.tuiInput)
	if err != nil {
		return err
	}

	transaction.ChartOfAccountsGroupName, err = utils.AssignStringField(f.ChartOfAccountsGroup, "chart of accounts group", f.tuiInput)
	if err != nil {
		return err
	}

	return nil
}

// processAmountFields handles amount and amount scale conversions
func (f *factoryTransactionCreate) processAmountFields(transaction *mmodel.CreateTransactionInput) error {
	if len(f.Amount) > 0 {
		amount, err := utils.StringToInt64(f.Amount)
		if err != nil {
			return err
		}

		transaction.Amount = &amount
	}

	if len(f.AmountScale) > 0 {
		amountScale, err := utils.StringToInt64(f.AmountScale)
		if err != nil {
			return err
		}

		transaction.AmountScale = &amountScale
	}

	return nil
}

// processSourceDestination handles source and destination account arrays
func (f *factoryTransactionCreate) processSourceDestination(transaction *mmodel.CreateTransactionInput) error {
	if len(f.Source) > 0 {
		var sources []string
		if err := json.Unmarshal([]byte(f.Source), &sources); err != nil {
			return errors.New("source must be a valid JSON array of strings")
		}

		transaction.Source = sources
	}

	if len(f.Destination) > 0 {
		var destinations []string
		if err := json.Unmarshal([]byte(f.Destination), &destinations); err != nil {
			return errors.New("destination must be a valid JSON array of strings")
		}

		transaction.Destination = destinations
	}

	return nil
}

// processStatusAndMetadata handles status, parent transaction ID and metadata
func (f *factoryTransactionCreate) processStatusAndMetadata(transaction *mmodel.CreateTransactionInput) error {
	if len(f.ParentTransactionID) > 0 {
		parentID := f.ParentTransactionID
		transaction.ParentTransactionID = &parentID
	}

	if len(f.StatusCode) > 0 {
		transaction.Status = &mmodel.Status{
			Code: f.StatusCode,
		}

		if len(f.StatusDescription) > 0 {
			description := f.StatusDescription
			transaction.Status.Description = &description
		}
	}

	if len(f.Metadata) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
			return errors.New("metadata must be a valid JSON object")
		}

		transaction.Metadata = metadata
	}

	return nil
}

func (f *factoryTransactionCreate) createRequestFromFlags(transaction *mmodel.CreateTransactionInput) error {
	if err := f.processBasicFields(transaction); err != nil {
		return err
	}

	if err := f.processAmountFields(transaction); err != nil {
		return err
	}

	if err := f.processSourceDestination(transaction); err != nil {
		return err
	}

	if err := f.processStatusAndMetadata(transaction); err != nil {
		return err
	}

	return nil
}

func (f *factoryTransactionCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.Description, "description", "", "Specify the transaction description.")
	cmd.Flags().StringVar(&f.Template, "template", "", "Specify the transaction template.")
	cmd.Flags().StringVar(&f.Amount, "amount", "", "Specify the transaction amount.")
	cmd.Flags().StringVar(&f.AmountScale, "amount-scale", "", "Specify the transaction amount scale.")
	cmd.Flags().StringVar(&f.AssetCode, "asset-code", "", "Specify the asset code (e.g., USD).")
	cmd.Flags().StringVar(&f.ChartOfAccountsGroup, "chart-of-accounts-group", "", "Specify the chart of accounts group.")
	cmd.Flags().StringVar(&f.Source, "source", "", "Specify the source accounts as a JSON array.")
	cmd.Flags().StringVar(&f.Destination, "destination", "", "Specify the destination accounts as a JSON array.")
	cmd.Flags().StringVar(&f.ParentTransactionID, "parent-transaction-id", "", "Specify the parent transaction ID.")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "", "Specify the status code.")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "", "Specify the status description.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "", "Specify metadata as a JSON object.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing the transaction data.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryTransactionCreate {
	return &factoryTransactionCreate{
		factory:         f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:        tui.Input,
	}
}

func newCmdTransactionCreate(f *factoryTransactionCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new transaction.",
		Long: utils.Format(
			"Creates a new transaction in the specified ledger.",
			"Returns the ID of the created transaction or an error message.",
		),
		Example: utils.Format(
			"$ mdz transaction create",
			"$ mdz transaction create -h",
			"$ mdz transaction create --organization-id <org-id> --ledger-id <ledger-id> --description \"Test transaction\" --template TRANSFER",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
