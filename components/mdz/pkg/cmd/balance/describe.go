package balance

import (
	"encoding/json"
	"fmt"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryBalanceDescribe struct {
	factory    *factory.Factory
	repoBalance repository.Balance
	tuiInput   func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID  string
	LedgerID        string
	BalanceID       string
	OutputFormat    string
}

func (f *factoryBalanceDescribe) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("balance-id") && len(f.BalanceID) < 1 {
		id, err := f.tuiInput("Enter your balance-id")
		if err != nil {
			return err
		}

		f.BalanceID = id
	}

	resp, err := f.repoBalance.GetByID(f.OrganizationID, f.LedgerID, f.BalanceID)
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
	table.Append([]string{"Account ID", resp.AccountID})
	
	// Format amount with scale
	formattedAmount := fmt.Sprintf("%d", resp.Amount)
	if resp.AmountScale > 0 {
		divisor := int64(1)
		for i := int64(0); i < resp.AmountScale; i++ {
			divisor *= 10
		}
		formattedAmount = fmt.Sprintf("%."+fmt.Sprintf("%d", resp.AmountScale)+"f", float64(resp.Amount)/float64(divisor))
	}
	
	table.Append([]string{"Amount", formattedAmount})
	table.Append([]string{"Amount (Raw)", fmt.Sprintf("%d", resp.Amount)})
	table.Append([]string{"Amount Scale", fmt.Sprintf("%d", resp.AmountScale)})
	table.Append([]string{"Asset Code", resp.AssetCode})
	table.Append([]string{"Organization ID", resp.OrganizationID})
	table.Append([]string{"Ledger ID", resp.LedgerID})
	
	// Format metadata
	if len(resp.Metadata) > 0 {
		metadataJSON, _ := json.MarshalIndent(resp.Metadata, "", "  ")
		table.Append([]string{"Metadata", string(metadataJSON)})
	}
	
	table.Append([]string{"Created At", resp.CreatedAt.Format("2006-01-02 15:04:05")})
	table.Append([]string{"Updated At", resp.UpdatedAt.Format("2006-01-02 15:04:05")})
	
	if resp.DeletedAt != nil {
		table.Append([]string{"Deleted At", resp.DeletedAt.Format("2006-01-02 15:04:05")})
	}
	
	table.Render()

	return nil
}

func (f *factoryBalanceDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.BalanceID, "balance-id", "", "Specify the balance ID.")
	cmd.Flags().StringVar(&f.OutputFormat, "output", "table", "Output format: table or json.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryBalanceDescribe {
	return &factoryBalanceDescribe{
		factory:    f,
		repoBalance: rest.NewBalance(f),
		tuiInput:   tui.Input,
	}
}

func newCmdBalanceDescribe(f *factoryBalanceDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes a balance.",
		Long: utils.Format(
			"Describes a specific balance in detail, including its properties",
			"and metadata. Returns a formatted table or JSON output",
			"depending on the specified output format.",
		),
		Example: utils.Format(
			"$ mdz balance describe",
			"$ mdz balance describe -h",
			"$ mdz balance describe --organization-id <org-id> --ledger-id <ledger-id> --balance-id <balance-id>",
			"$ mdz balance describe --output json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
