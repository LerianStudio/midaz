package assetrate

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

	"github.com/spf13/cobra"
)

type factoryAssetRateDescribe struct {
	factory       *factory.Factory
	repoAssetRate repository.AssetRate
	tuiInput      func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID string
	LedgerID       string
	AssetRateID    string
	OutputFormat   string
}

// validateAndGetInputs validates the required inputs and prompts for missing ones
func (f *factoryAssetRateDescribe) validateAndGetInputs(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("asset-rate-id") && len(f.AssetRateID) < 1 {
		id, err := f.tuiInput("Enter your asset-rate-id")
		if err != nil {
			return err
		}

		f.AssetRateID = id
	}

	return nil
}

func (f *factoryAssetRateDescribe) runE(cmd *cobra.Command, _ []string) error {
	if err := f.validateAndGetInputs(cmd); err != nil {
		return err
	}

	resp, err := f.repoAssetRate.GetByID(f.OrganizationID, f.LedgerID, f.AssetRateID)
	if err != nil {
		return err
	}

	// JSON output format
	if f.OutputFormat == "json" {
		jsonData, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}

		output.Printf(f.factory.IOStreams.Out, string(jsonData))

		return nil
	}

	// Default output format (table)
	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"Property", "Value"})

	table.Append([]string{"ID", resp.ID})
	table.Append([]string{"From Asset Code", resp.FromAssetCode})
	table.Append([]string{"To Asset Code", resp.ToAssetCode})
	table.Append([]string{"Rate (Raw)", strconv.FormatInt(resp.Rate, 10)})
	table.Append([]string{"Rate Scale", strconv.FormatInt(resp.RateScale, 10)})

	if resp.Status != nil {
		table.Append([]string{"Status Code", resp.Status.Code})

		if resp.Status.Description != nil {
			table.Append([]string{"Status Description", *resp.Status.Description})
		}
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

	table.Render()

	return nil
}

func (f *factoryAssetRateDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AssetRateID, "asset-rate-id", "", "Specify the asset rate ID.")
	cmd.Flags().StringVar(&f.OutputFormat, "output", "table", "Output format. One of: table|json")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryAssetRateDescribe {
	return &factoryAssetRateDescribe{
		factory:       f,
		repoAssetRate: rest.NewAssetRate(f),
		tuiInput:      tui.Input,
	}
}

func newCmdAssetRateDescribe(f *factoryAssetRateDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes an asset rate.",
		Long: utils.Format(
			"Describes an asset rate by its ID.",
			"Returns detailed information about the asset rate or an error message.",
		),
		Example: utils.Format(
			"$ mdz asset-rate describe",
			"$ mdz asset-rate describe -h",
			"$ mdz asset-rate describe --organization-id <org-id> --ledger-id <ledger-id> --asset-rate-id <asset-rate-id>",
			"$ mdz asset-rate describe --output json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
