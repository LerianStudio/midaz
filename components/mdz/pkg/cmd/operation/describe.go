package operation

import (
	"encoding/json"
	"fmt"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"strconv"

	"github.com/spf13/cobra"
)

type factoryOperationDescribe struct {
	factory       *factory.Factory
	repoOperation repository.Operation
	tuiInput      func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID string
	LedgerID       string
	OperationID    string
	OutputFormat   string
}

func (f *factoryOperationDescribe) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("operation-id") && len(f.OperationID) < 1 {
		id, err := f.tuiInput("Enter your operation-id")
		if err != nil {
			return err
		}

		f.OperationID = id
	}

	resp, err := f.repoOperation.GetByID(f.OrganizationID, f.LedgerID, f.OperationID)
	if err != nil {
		return err
	}

	if f.OutputFormat == "json" {
		jsonData, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling JSON: %v", err)
		}

		output.Printf(f.factory.IOStreams.Out, "%s", string(jsonData))

		return nil
	}

	// Default output format (table)
	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"Property", "Value"})

	table.Append([]string{"ID", resp.ID})
	table.Append([]string{"Transaction ID", resp.TransactionID})
	table.Append([]string{"Account ID", resp.AccountID})
	table.Append([]string{"Type", resp.Type})
	table.Append([]string{"Amount", strconv.FormatInt(resp.Amount, 10)})
	table.Append([]string{"Asset Code", resp.AssetCode})

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

	table.Render()

	return nil
}

func (f *factoryOperationDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.OperationID, "operation-id", "", "Specify the operation ID.")
	cmd.Flags().StringVar(&f.OutputFormat, "output", "table", "Output format: table or json.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryOperationDescribe {
	return &factoryOperationDescribe{
		factory:       f,
		repoOperation: rest.NewOperation(f),
		tuiInput:      tui.Input,
	}
}

func newCmdOperationDescribe(f *factoryOperationDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes an operation.",
		Long: utils.Format(
			"Describes a specific operation in detail, including its properties",
			"and metadata. Returns a formatted table or JSON output",
			"depending on the specified output format.",
		),
		Example: utils.Format(
			"$ mdz operation describe",
			"$ mdz operation describe -h",
			"$ mdz operation describe --organization-id <org-id> --ledger-id <ledger-id> --operation-id <op-id>",
			"$ mdz operation describe --output json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
