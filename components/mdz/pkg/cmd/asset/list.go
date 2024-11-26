package asset

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryAssetList struct {
	factory        *factory.Factory
	repoAsset      repository.Asset
	OrganizationID string
	LedgerID       string
	Limit          int
	Page           int
	JSON           bool
}

func (f *factoryAssetList) runE(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := tui.Input("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.LedgerID = id
	}

	leds, err := f.repoAsset.Get(f.OrganizationID, f.LedgerID, f.Limit, f.Page)
	if err != nil {
		return err
	}

	if f.JSON {
		b, err := json.Marshal(leds)
		if err != nil {
			return err
		}

		output.Printf(f.factory.IOStreams.Out, string(b))

		return nil
	}

	headerFmt := color.New(color.FgYellow).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	tbl := table.New(
		"ID",
		"NAME",
		"TYPE",
		"CODE",
		"STATUS_CODE",
		"STATUS_DESCRIPTION",
		"CREATED_AT",
	)

	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
	tbl.WithWriter(f.factory.IOStreams.Out)

	for _, i := range leds.Items {
		tbl.AddRow(
			i.ID,
			i.Name,
			i.Type,
			i.Code,
			i.Status.Code,
			utils.SafeString(i.Status.Description),
			i.CreatedAt,
		)
	}

	tbl.Print()

	return nil
}

func (f *factoryAssetList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().IntVar(&f.Limit, "limit", 10,
		"Specifies the number of ledgers to retrieve per page")
	cmd.Flags().IntVar(&f.Page, "page", 1,
		"Specifies the page number for paginated results")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryAssetList {
	return &factoryAssetList{
		factory:   f,
		repoAsset: rest.NewAsset(f),
	}
}

func newCmdAssetList(f *factoryAssetList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Displays all registered assets.",
		Long: utils.Format(
			"Lists all the assets allowed in the ledger, showing a complete ",
			"overview of currencies, commodities and registered goods. This list",
			"makes it easy to see which assets are available for transactions",
			"and balances.",
		),
		Example: utils.Format(
			"$ mdz asset list",
			"$ mdz asset list -h",
			"$ mdz asset list --limit 10",
			"$ mdz asset list --organization-id 43212 --ledger-id 12333",
			"$ mdz asset list --organization-id 43212 --ledger-id 12333 --page 3",
			"$ mdz asset list --organization-id 43212 --ledger-id 12333 --limit 10 --page 2",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
