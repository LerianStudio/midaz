package assetrate

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryAssetRateList struct {
	factory       *factory.Factory
	repoAssetRate repository.AssetRate
	tuiInput      func(message string) (string, error)
	flagsListAll
}

type flagsListAll struct {
	OrganizationID string
	LedgerID       string
	AssetCode      string
	Page           int
	Limit          int
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryAssetRateList) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("asset-code") && len(f.AssetCode) < 1 {
		code, err := f.tuiInput("Enter the asset code (e.g., USD)")
		if err != nil {
			return errors.Wrap(err, "failed to get asset code from input")
		}

		f.AssetCode = code
	}

	assetRates, err := f.repoAssetRate.GetByAssetCode(
		f.OrganizationID, f.LedgerID, f.AssetCode,
		f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return errors.CommandError("assetrate list", err)
	}

	output.FormatAndPrint(f.factory, assetRates, "", "")

	return nil
}

func (f *factoryAssetRateList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AssetCode, "asset-code", "", "Source asset code to list rates for (e.g., USD).")
	cmd.Flags().IntVar(&f.Page, "page", 1, "Page number for pagination.")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Number of items per page for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "", "Sorting order for results (ASC or DESC).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Start date filter (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "End date filter (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryAssetRateList {
	return &factoryAssetRateList{
		factory:       f,
		repoAssetRate: rest.NewAssetRate(f),
		tuiInput:      tui.Input,
	}
}

func newCmdAssetRateList(f *factoryAssetRateList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists asset rates.",
		Long: utils.Format(
			"Lists asset rates for a specific source asset code. Returns a",
			"list of rates with pagination support.",
		),
		Example: utils.Format(
			"$ mdz assetrate list",
			"$ mdz assetrate list -h",
			"$ mdz assetrate list --organization-id 123 --ledger-id 456 --asset-code USD",
			"$ mdz assetrate list --asset-code EUR --limit 20 --page 2",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
