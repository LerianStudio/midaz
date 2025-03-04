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
	Idempotency                string
	JSONFile                   string
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
	if f.Description != "" {
		transaction.Description = f.Description
	} else {
		transaction.Description, err = f.tuiInput("description")
		if err != nil {
			return errors.Wrap(err, "failed to assign description field")
		}
	}

	// Get chart of accounts group name
	if f.ChartOfAccountsGroupName != "" {
		transaction.ChartOfAccountsGroupName = f.ChartOfAccountsGroupName
	} else {
		transaction.ChartOfAccountsGroupName, err = f.tuiInput("chart-of-accounts-group-name")
		if err != nil {
			return errors.Wrap(err, "failed to assign chart of accounts group name field")
		}
	}

	// Parse metadata
	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.ValidationError("metadata", "Invalid JSON format for metadata")
	}
	transaction.Metadata = metadata

	// Create transaction structure
	var asset string
	if f.Asset != "" {
		asset = f.Asset
	} else {
		asset, err = f.tuiInput("asset")
		if err != nil {
			return errors.Wrap(err, "failed to assign asset field")
		}
	}

	scale := f.Scale
	value := f.Value

	transaction.Send = &mmodel.TransactionSend{
		Asset: asset,
		Value: value,
		Scale: scale,
	}

	// Get source account
	var sourceAccount string
	if f.SourceAccount != "" {
		sourceAccount = f.SourceAccount
	} else {
		sourceAccount, err = f.tuiInput("source-account")
		if err != nil {
			return errors.Wrap(err, "failed to assign source account field")
		}
	}

	// Get source chart of accounts
	var sourceChartOfAccounts string
	if f.SourceChartOfAccounts != "" {
		sourceChartOfAccounts = f.SourceChartOfAccounts
	} else {
		sourceChartOfAccounts, err = f.tuiInput("source-chart-of-accounts")
		if err != nil {
			return errors.Wrap(err, "failed to assign source chart of accounts field")
		}
	}

	// Get source description
	var sourceDescription string
	if f.SourceDescription != "" {
		sourceDescription = f.SourceDescription
	} else {
		sourceDescription, err = f.tuiInput("source-description")
		if err != nil {
			return errors.Wrap(err, "failed to assign source description field")
		}
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
	var destinationAccount string
	if f.DestinationAccount != "" {
		destinationAccount = f.DestinationAccount
	} else {
		destinationAccount, err = f.tuiInput("destination-account")
		if err != nil {
			return errors.Wrap(err, "failed to assign destination account field")
		}
	}

	// Get destination chart of accounts
	var destinationChartOfAccounts string
	if f.DestinationChartOfAccounts != "" {
		destinationChartOfAccounts = f.DestinationChartOfAccounts
	} else {
		destinationChartOfAccounts, err = f.tuiInput("destination-chart-of-accounts")
		if err != nil {
			return errors.Wrap(err, "failed to assign destination chart of accounts field")
		}
	}

	// Get destination description
	var destinationDescription string
	if f.DestinationDescription != "" {
		destinationDescription = f.DestinationDescription
	} else {
		destinationDescription, err = f.tuiInput("destination-description")
		if err != nil {
			return errors.Wrap(err, "failed to assign destination description field")
		}
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
	if len(f.Idempotency) > 0 {
		transaction.Idempotency = f.Idempotency
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
		transaction.Idempotency = hex.EncodeToString(h.Sum(nil))
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
	cmd.Flags().StringVar(&f.Idempotency, "idempotency", "", "Unique identifier to prevent duplicate transactions. If not provided, one will be generated.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing transaction attributes, or '-' for stdin.")

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
