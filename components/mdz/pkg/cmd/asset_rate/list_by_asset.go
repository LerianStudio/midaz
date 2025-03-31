package asset_rate

import (
	"fmt"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"strconv"

	"github.com/spf13/cobra"
)

type factoryAssetRateListByAsset struct {
	factory       *factory.Factory
	repoAssetRate repository.AssetRate
	tuiInput      func(message string) (string, error)
	flagsListByAsset
}

type flagsListByAsset struct {
	OrganizationID string
	LedgerID       string
	AssetCode      string
	Limit          int
	Page           int
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryAssetRateListByAsset) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("asset-code") && len(f.AssetCode) < 1 {
		code, err := f.tuiInput("Enter the asset code")
		if err != nil {
			return err
		}

		f.AssetCode = code
	}

	resp, err := f.repoAssetRate.GetByAssetCode(f.OrganizationID, f.LedgerID, f.AssetCode, f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return err
	}

	f.printAssetRates(resp)

	return nil
}

func (f *factoryAssetRateListByAsset) printAssetRates(assetRates *mmodel.AssetRates) {
	if len(assetRates.Items) == 0 {
		output.Printf(f.factory.IOStreams.Out, "No asset rates found for asset code %s", f.AssetCode)
		return
	}

	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"ID", "From Asset", "To Asset", "Rate", "Status", "Created At"})

	for _, ar := range assetRates.Items {
		// Format rate with scale
		formattedRate := strconv.FormatInt(ar.Rate, 10)

		if ar.RateScale > 0 {
			divisor := int64(1)
			for i := int64(0); i < ar.RateScale; i++ {
				divisor *= 10
			}

			formattedRate = fmt.Sprintf("%."+strconv.FormatInt(ar.RateScale, 10)+"f", float64(ar.Rate)/float64(divisor))
		}

		statusCode := ""
		if ar.Status != nil {
			statusCode = ar.Status.Code
		}

		table.Append([]string{
			ar.ID,
			ar.FromAssetCode,
			ar.ToAssetCode,
			formattedRate,
			statusCode,
			ar.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	table.Render()

	if assetRates.Pagination != nil {
		output.Printf(f.factory.IOStreams.Out, "\nPage: %d, Total: %d", f.Page, len(assetRates.Items))

		if f.Page > 1 {
			output.Printf(f.factory.IOStreams.Out, ", Previous page: mdz asset-rate list-by-asset --organization-id %s --ledger-id %s --asset-code %s --page %d",
				f.OrganizationID, f.LedgerID, f.AssetCode, f.Page-1)
		}

		if len(assetRates.Items) == f.Limit {
			output.Printf(f.factory.IOStreams.Out, ", Next page: mdz asset-rate list-by-asset --organization-id %s --ledger-id %s --asset-code %s --page %d",
				f.OrganizationID, f.LedgerID, f.AssetCode, f.Page+1)
		}
	}
}

func (f *factoryAssetRateListByAsset) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AssetCode, "asset-code", "", "Specify the asset code (e.g., USD).")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Limit the number of asset rates returned.")
	cmd.Flags().IntVar(&f.Page, "page", 1, "Specify the page number for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "desc", "Sort order (asc or desc).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Filter by start date (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "Filter by end date (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacListByAsset(f *factory.Factory) *factoryAssetRateListByAsset {
	return &factoryAssetRateListByAsset{
		factory:       f,
		repoAssetRate: rest.NewAssetRate(f),
		tuiInput:      tui.Input,
	}
}

func newCmdAssetRateListByAsset(f *factoryAssetRateListByAsset) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-by-asset",
		Short: "Lists asset rates for a specific asset.",
		Long: utils.Format(
			"Lists all asset rates for a specific asset code in the specified ledger.",
			"The results can be filtered and paginated using the available flags.",
			"Returns a table of asset rates or an error message.",
		),
		Example: utils.Format(
			"$ mdz asset-rate list-by-asset",
			"$ mdz asset-rate list-by-asset -h",
			"$ mdz asset-rate list-by-asset --organization-id <org-id> --ledger-id <ledger-id> --asset-code USD",
			"$ mdz asset-rate list-by-asset --limit 20 --page 2 --sort-order asc",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
