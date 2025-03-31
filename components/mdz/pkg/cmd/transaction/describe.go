package transaction

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/spf13/cobra"
)

type factoryTransactionDescribe struct {
	factory         *factory.Factory
	repoTransaction repository.Transaction
	tuiInput        func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID string
	LedgerID       string
	TransactionID  string
	OutputFormat   string
}

// validateAndGetInputs validates the required inputs and prompts for missing ones
func (f *factoryTransactionDescribe) validateAndGetInputs(cmd *cobra.Command) error {
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

	return nil
}

// renderTransactionTable renders the transaction details in a table format
func (f *factoryTransactionDescribe) renderTransactionTable(resp *mmodel.Transaction) error {
	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"Property", "Value"})

	table.Append([]string{"ID", resp.ID})
	table.Append([]string{"Description", resp.Description})
	table.Append([]string{"Template", resp.Template})

	if resp.Amount != nil {
		table.Append([]string{"Amount", strconv.FormatInt(*resp.Amount, 10)})
	}

	if resp.AmountScale != nil {
		table.Append([]string{"Amount Scale", strconv.FormatInt(*resp.AmountScale, 10)})
	}

	table.Append([]string{"Asset Code", resp.AssetCode})
	table.Append([]string{"Chart of Accounts Group", resp.ChartOfAccountsGroupName})

	if resp.ParentTransactionID != nil {
		table.Append([]string{"Parent Transaction ID", *resp.ParentTransactionID})
	}

	if resp.Status != nil {
		table.Append([]string{"Status Code", resp.Status.Code})

		if resp.Status.Description != nil {
			table.Append([]string{"Status Description", *resp.Status.Description})
		}
	}

	// Format source accounts
	if len(resp.Source) > 0 {
		sourceJSON, err := json.Marshal(resp.Source)
		if err != nil {
			return fmt.Errorf("error marshaling source accounts: %w", err)
		}

		table.Append([]string{"Source Accounts", string(sourceJSON)})
	}

	// Format destination accounts
	if len(resp.Destination) > 0 {
		destJSON, err := json.Marshal(resp.Destination)
		if err != nil {
			return fmt.Errorf("error marshaling destination accounts: %w", err)
		}

		table.Append([]string{"Destination Accounts", string(destJSON)})
	}

	// Format metadata
	if len(resp.Metadata) > 0 {
		metadataJSON, err := json.MarshalIndent(resp.Metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling metadata: %w", err)
		}

		table.Append([]string{"Metadata", string(metadataJSON)})
	}

	table.Append([]string{"Created At", resp.CreatedAt.Format("2006-01-02 15:04:05")})
	table.Append([]string{"Updated At", resp.UpdatedAt.Format("2006-01-02 15:04:05")})

	if resp.DeletedAt != nil {
		table.Append([]string{"Deleted At", resp.DeletedAt.Format("2006-01-02 15:04:05")})
	}

	// Display operations if available
	if len(resp.Operations) > 0 {
		output.Printf(f.factory.IOStreams.Out, "\nOperations:\n")
		opTable := output.NewTable(f.factory.IOStreams.Out)
		opTable.SetHeader([]string{"ID", "Account ID", "Type", "Amount", "Asset Code"})

		for _, op := range resp.Operations {
			opTable.Append([]string{
				op.ID,
				op.AccountID,
				op.Type,
				strconv.FormatInt(op.Amount, 10),
				op.AssetCode,
			})
		}

		opTable.Render()
	}

	table.Render()

	return nil
}

func (f *factoryTransactionDescribe) runE(cmd *cobra.Command, _ []string) error {
	if err := f.validateAndGetInputs(cmd); err != nil {
		return err
	}

	resp, err := f.repoTransaction.GetByID(f.OrganizationID, f.LedgerID, f.TransactionID)
	if err != nil {
		return err
	}

	if f.OutputFormat == "json" {
		jsonData, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling JSON: %w", err)
		}

		output.Printf(f.factory.IOStreams.Out, "%s", string(jsonData))

		return nil
	}

	return f.renderTransactionTable(resp)
}

func (f *factoryTransactionDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.TransactionID, "transaction-id", "", "Specify the transaction ID.")
	cmd.Flags().StringVar(&f.OutputFormat, "output", "table", "Output format: table or json.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryTransactionDescribe {
	return &factoryTransactionDescribe{
		factory:         f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:        tui.Input,
	}
}

func newCmdTransactionDescribe(f *factoryTransactionDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes a transaction.",
		Long: utils.Format(
			"Describes a specific transaction in detail, including its properties,",
			"operations, and metadata. Returns a formatted table or JSON output",
			"depending on the specified output format.",
		),
		Example: utils.Format(
			"$ mdz transaction describe",
			"$ mdz transaction describe -h",
			"$ mdz transaction describe --organization-id <org-id> --ledger-id <ledger-id> --transaction-id <tx-id>",
			"$ mdz transaction describe --output json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
