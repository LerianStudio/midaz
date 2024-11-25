package account

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

type factoryAccountList struct {
	factory        *factory.Factory
	repoAccount    repository.Account
	OrganizationID string
	LedgerID       string
	PortfolioID    string
	Limit          int
	Page           int
	JSON           bool
}

func (f *factoryAccountList) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("portfolio-id") && len(f.PortfolioID) < 1 {
		id, err := tui.Input("Enter your portfolio-id")
		if err != nil {
			return err
		}

		f.PortfolioID = id
	}

	accounts, err := f.repoAccount.Get(f.OrganizationID, f.LedgerID, f.PortfolioID, f.Limit, f.Page)
	if err != nil {
		return err
	}

	if f.JSON {
		b, err := json.Marshal(accounts)
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
		"STATUS_CODE",
		"STATUS_DESCRIPTION",
		"CREATED_AT",
	)

	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
	tbl.WithWriter(f.factory.IOStreams.Out)

	for _, i := range accounts.Items {
		tbl.AddRow(
			i.ID,
			i.Name,
			i.Type,
			i.Status.Code,
			utils.SafeString(i.Status.Description),
			i.CreatedAt,
		)
	}

	tbl.Print()

	return nil
}

func (f *factoryAccountList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID.")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().IntVar(&f.Limit, "limit", 10,
		"Specifies the number of ledgers to retrieve per page")
	cmd.Flags().IntVar(&f.Page, "page", 1,
		"Specifies the page number for paginated results")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryAccountList {
	return &factoryAccountList{
		factory:     f,
		repoAccount: rest.NewAccount(f),
	}
}

func newCmdAccountList(f *factoryAccountList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists all accounts.",
		Long: utils.Format(
			"Returns a list of all accounts associated with the current portfolio.",
		),
		Example: utils.Format(
			"$ mdz account list",
			"$ mdz account list -h",
			"$ mdz account list --limit 10",
			"$ mdz account list --organization-id 43212 --ledger-id 12333 --portfolio-id 43444",
			"$ mdz account list --organization-id 43212 --ledger-id 12333 --portfolio-id 43444 --page 3",
			"$ mdz account list --organization-id 43212 --ledger-id 12333 --portfolio-id 43444 --limit 10 --page 2",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
