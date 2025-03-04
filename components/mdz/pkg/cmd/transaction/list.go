package transaction

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryTransactionList struct {
	factory        *factory.Factory
	repoTransaction repository.Transaction
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	Page           int
	Limit          int
	SortOrder      string
	StartDate      string
	EndDate        string
	JSON           bool
}

func (f *factoryTransactionList) ensureFlagInput(cmd *cobra.Command) error {
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

	if len(f.StartDate) > 0 {
		if err := utils.ValidateDate(f.StartDate); err != nil {
			return errors.ValidationError("start-date", "invalid date format, use YYYY-MM-DD")
		}
	}

	if len(f.EndDate) > 0 {
		if err := utils.ValidateDate(f.EndDate); err != nil {
			return errors.ValidationError("end-date", "invalid date format, use YYYY-MM-DD")
		}
	}

	return nil
}

func (f *factoryTransactionList) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	transactions, err := f.repoTransaction.Get(
		f.OrganizationID,
		f.LedgerID,
		f.Limit,
		f.Page,
		f.SortOrder,
		f.StartDate,
		f.EndDate,
	)
	if err != nil {
		return errors.CommandError("transaction list", err)
	}

	if f.JSON {
		b, err := json.Marshal(transactions)
		if err != nil {
			return errors.Wrap(err, "failed to marshal transactions to JSON")
		}

		output.Printf(f.factory.IOStreams.Out, string(b))

		return nil
	}

	tbl := table.New(
		"ID",
		"TYPE",
		"AMOUNT",
		"STATUS",
		"SOURCE_ACCOUNT",
		"DESTINATION_ACCOUNT",
		"CREATED_AT",
	)

	if !f.factory.NoColor {
		headerFmt := color.New(color.FgYellow).SprintfFunc()
		fieldFmt := color.New(color.FgYellow).SprintfFunc()
		tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	}

	tbl.WithWriter(f.factory.IOStreams.Out)

	for _, tx := range transactions.Data {
		tbl.AddRow(
			tx.ID,
			tx.Type,
			string(tx.Amount)+" "+tx.Currency,
			tx.Status,
			tx.SourceAccountID,
			tx.DestinationAccountID,
			tx.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}

	tbl.Print()

	return nil
}

func (f *factoryTransactionList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Specify the number of items per page.")
	cmd.Flags().IntVar(&f.Page, "page", 1, "Specify the page number for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "desc", "Specify the sort order (asc or desc).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Filter by start date (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "Filter by end date (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryTransactionList {
	return &factoryTransactionList{
		factory:        f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:       tui.Input,
	}
}

func newCmdTransactionList(f *factoryTransactionList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists transactions in a ledger.",
		Long: utils.Format(
			"Lists all transactions in the specified ledger with optional",
			"filtering and pagination. Returns a table of transactions with",
			"essential details.",
		),
		Example: utils.Format(
			"$ mdz transaction list",
			"$ mdz transaction list -h",
			"$ mdz transaction list --organization-id org_123 --ledger-id ldg_456",
			"$ mdz transaction list --limit 20 --page 2 --sort-order asc",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}