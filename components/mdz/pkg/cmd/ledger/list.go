package ledger

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

type factoryLedgerList struct {
	factory        *factory.Factory
	repoLedger     repository.Ledger
	OrganizationID string
	Limit          int
	Page           int
	JSON           bool
}

func (f *factoryLedgerList) runE(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	leds, err := f.repoLedger.Get(f.OrganizationID, f.Limit, f.Page)
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
			i.Status.Code,
			utils.SafeString(i.Status.Description),
			i.CreatedAt,
		)
	}

	tbl.Print()

	return nil
}

func (f *factoryLedgerList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().IntVar(&f.Limit, "limit", 10,
		"Specifies the number of ledgers to retrieve per page")
	cmd.Flags().IntVar(&f.Page, "page", 1,
		"Specifies the page number for paginated results")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryLedgerList {
	return &factoryLedgerList{
		factory:    f,
		repoLedger: rest.NewLedger(f),
	}
}

func newCmdLedgerList(f *factoryLedgerList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists all the ledgers in an organization",
		Long: `Displays all ledgers associated with the organization, allowing 
			you to view and verification of each transaction record.`,
		Example: utils.Format(
			"$ mdz ledger list",
			"$ mdz ledger list -h",
			"$ mdz ledger list --limit 10",
			"$ mdz ledger list --page 3",
			"$ mdz ledger list --limit 10 --page 2",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}