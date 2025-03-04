// Package transaction provides commands for managing transactions in the Midaz CLI
package transaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

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

type factoryTransactionCreate struct {
	factory         *factory.Factory
	repoTransaction repository.Transaction
	tuiInput        func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID             string
	LedgerID                   string
	ChartOfAccountsGroupName   string
	Description                string
	Asset                      string
	Value                      int64
	Scale                      int
	SourceAccount              string
	SourceChartOfAccounts      string
	SourceDescription          string
	DestinationAccount         string
	DestinationChartOfAccounts string
	DestinationDescription     string
	Metadata                   string
	IdempotencyKey             string
	JSONFile                   string
	// Legacy fields
	Type                 string
	Status               string
	Amount               string
	Currency             string
	SourceAccountID      string
	DestinationAccountID string
}

func (f *factoryTransactionCreate) runE(cmd *cobra.Command, _ []string) error {
	transaction := mmodel.CreateTransactionInput{}

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

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &transaction)
		if err != nil {
			return errors.UserError(err, "Verify if the file format is JSON or fix its content according to the JSON format specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&transaction)
		if err != nil {
			return errors.Wrap(err, "failed to create transaction request from flags")
		}
	}

	resp, err := f.repoTransaction.Create(f.OrganizationID, f.LedgerID, transaction)
	if err != nil {
		return errors.CommandError("transaction create", err)
	}

	output.FormatAndPrint(f.factory, resp.ID, "Transaction", output.Created)

	return nil
}

func (f *factoryTransactionCreate) createRequestFromFlags(transaction *mmodel.CreateTransactionInput) error {
	var err error

	// Get description
	transaction.Description, err = utils.AssignStringField(f.Description, "description", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign description field")
	}

	// Get chart of accounts group name
	transaction.ChartOfAccountsGroupName, err = utils.AssignStringField(f.ChartOfAccountsGroupName, "chart-of-accounts-group-name", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign chart of accounts group name field")
	}

	// Parse metadata
	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.ValidationError("metadata", "Invalid JSON format for metadata")
	}
	transaction.Metadata = metadata

	// Create transaction structure
	asset, err := utils.AssignStringField(f.Asset, "asset", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign asset field")
	}

	// Set default scale if not provided
	scale := f.Scale
	if scale == 0 {
		scale = 0 // Default scale
	}

	// Create transaction send structure with integer value
	// Default to 100 if no value is provided
	value := f.Value
	if value == 0 {
		value = 100
	}

	transaction.Send = &mmodel.TransactionSend{
		Asset: asset,
		Value: value,
		Scale: scale,
	}

	// Get source account
	sourceAccount, err := utils.AssignStringField(f.SourceAccount, "source-account", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign source account field")
	}

	// Get source chart of accounts
	sourceChartOfAccounts, err := utils.AssignStringField(f.SourceChartOfAccounts, "source-chart-of-accounts", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign source chart of accounts field")
	}

	// Get source description
	sourceDescription, err := utils.AssignStringField(f.SourceDescription, "source-description", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign source description field")
	}

	// Create source operation
	sourceOperation := &mmodel.TransactionOperation{
		Account:         sourceAccount,
		Description:     sourceDescription,
		ChartOfAccounts: sourceChartOfAccounts,
		Amount: &mmodel.TransactionAmount{
			Asset: asset,
			Value: value,
			Scale: scale,
		},
		Metadata: metadata,
	}

	// Create source
	transaction.Send.Source = &mmodel.TransactionSource{
		From: []*mmodel.TransactionOperation{sourceOperation},
	}

	// Get destination account
	destinationAccount, err := utils.AssignStringField(f.DestinationAccount, "destination-account", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign destination account field")
	}

	// Get destination chart of accounts
	destinationChartOfAccounts, err := utils.AssignStringField(f.DestinationChartOfAccounts, "destination-chart-of-accounts", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign destination chart of accounts field")
	}

	// Get destination description
	destinationDescription, err := utils.AssignStringField(f.DestinationDescription, "destination-description", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign destination description field")
	}

	// Create destination operation
	destinationOperation := &mmodel.TransactionOperation{
		Account:         destinationAccount,
		Description:     destinationDescription,
		ChartOfAccounts: destinationChartOfAccounts,
		Amount: &mmodel.TransactionAmount{
			Asset: asset,
			Value: value,
			Scale: scale,
		},
		Metadata: metadata,
	}

	// Create distribute
	transaction.Send.Distribute = &mmodel.TransactionDistribute{
		To: []*mmodel.TransactionOperation{destinationOperation},
	}

	// Set idempotency key or generate a unique one
	if len(f.IdempotencyKey) > 0 {
		transaction.IdempotencyKey = f.IdempotencyKey
	} else {
		// Generate a unique key based on the transaction details and current time
		now := time.Now().Format(time.RFC3339Nano)
		keyData := fmt.Sprintf("%s-%s-%s-%s-%d-%s-%s",
			transaction.Description,
			asset,
			sourceAccount,
			destinationAccount,
			value,
			now,
			// Add some randomness
			fmt.Sprintf("%d", time.Now().UnixNano()),
		)
		
		// Generate SHA-256 hash
		h := sha256.New()
		h.Write([]byte(keyData))
		transaction.IdempotencyKey = hex.EncodeToString(h.Sum(nil))
	}
	
	// Backward compatibility - these fields will be ignored by the API
	if len(f.Type) > 0 {
		transaction.Type = f.Type
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
	if len(f.SourceAccountID) > 0 {
		transaction.SourceAccountID = f.SourceAccountID
	}
	if len(f.DestinationAccountID) > 0 {
		transaction.DestinationAccountID = f.DestinationAccountID
	}

	return nil
}

func (f *factoryTransactionCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.ChartOfAccountsGroupName, "chart-group", "", "Specify the chart of accounts group name.")
	cmd.Flags().StringVar(&f.Description, "description", "", "Provide a description for the transaction.")
	cmd.Flags().StringVar(&f.Asset, "asset", "", "Specify the asset code (e.g., BRL, USD).")
	cmd.Flags().Int64Var(&f.Value, "value", 100, "Specify the transaction value.")
	cmd.Flags().IntVar(&f.Scale, "scale", 0, "Specify the decimal scale for the value.")
	cmd.Flags().StringVar(&f.SourceAccount, "source-account", "", "Specify the source account.")
	cmd.Flags().StringVar(&f.SourceChartOfAccounts, "source-chart", "", "Specify the source chart of accounts.")
	cmd.Flags().StringVar(&f.SourceDescription, "source-description", "", "Provide a description for the source operation.")
	cmd.Flags().StringVar(&f.DestinationAccount, "destination-account", "", "Specify the destination account.")
	cmd.Flags().StringVar(&f.DestinationChartOfAccounts, "destination-chart", "", "Specify the destination chart of accounts.")
	cmd.Flags().StringVar(&f.DestinationDescription, "destination-description", "", "Provide a description for the destination operation.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.IdempotencyKey, "idempotency-key", "", "Unique identifier to prevent duplicate transactions. If not provided, one will be generated.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing transaction attributes, or '-' for stdin.")

	// Legacy flags - kept for backward compatibility
	cmd.Flags().StringVar(&f.Type, "type", "", "Specify the transaction type (e.g., TRANSFER, DEPOSIT, WITHDRAWAL). [Deprecated]")
	cmd.Flags().StringVar(&f.Status, "status", "", "Specify the status of the transaction (e.g., PENDING, COMPLETED). [Deprecated]")
	cmd.Flags().StringVar(&f.Amount, "amount", "", "Specify the transaction amount. [Deprecated]")
	cmd.Flags().StringVar(&f.Currency, "currency", "", "Specify the currency code (e.g., USD). [Deprecated]")
	cmd.Flags().StringVar(&f.SourceAccountID, "source-account-id", "", "Specify the source account ID. [Deprecated]")
	cmd.Flags().StringVar(&f.DestinationAccountID, "destination-account-id", "", "Specify the destination account ID. [Deprecated]")

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
		Short: "Creates a transaction.",
		Long: utils.Format(
			"Creates a new transaction in the specified ledger. Allows for transfers,",
			"deposits, or withdrawals between accounts using the standard Midaz transaction format.",
			"This command supports both the JSON API format and the command-line flags for creating transactions.",
			"The transaction uses a 'send' structure specifying asset, value, scale, source account(s) and destination account(s).",
			"Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz transaction create",
			"$ mdz transaction create -h",
			"$ mdz transaction create --organization-id org123 --ledger-id ldg123 --chart-group PIX_TRANSACTIONS",
			"    --description \"New Transaction\" --asset BRL --value 100",
			"    --source-account @external/BRL --source-chart PIX_DEBIT --source-description \"Debit Operation\"",
			"    --destination-account @account1_BRL --destination-chart PIX_CREDIT --destination-description \"Credit Operation\"",
			"$ mdz transaction create --json-file payload.json",
			"$ cat payload.json | mdz transaction create --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
