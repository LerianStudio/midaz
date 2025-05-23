package balance

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

type factoryBalanceList struct {
	factory     *factory.Factory
	repoBalance repository.Balance
	tuiInput    func(message string) (string, error)
	flagsList
}

type flagsList struct {
	OrganizationID string
	LedgerID       string
	Limit          int
	Cursor         string
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryBalanceList) runE(cmd *cobra.Command, _ []string) error {
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

	resp, err := f.repoBalance.Get(f.OrganizationID, f.LedgerID, f.Limit, f.Cursor, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return err
	}

	f.printBalances(resp)

	return nil
}

func (f *factoryBalanceList) printBalances(balances *mmodel.Balances) {
	if len(balances.Items) == 0 {
		output.Printf(f.factory.IOStreams.Out, "No balances found")
		return
	}

	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"ID", "Account ID", "Available", "On Hold", "Asset Code", "Created At"})

	for _, b := range balances.Items {
		// Format available amount with scale
		formattedAvailable := strconv.FormatInt(b.Available, 10)

		if b.Scale > 0 {
			divisor := int64(1)

			for i := int64(0); i < b.Scale; i++ {
				divisor *= 10
			}

			formattedAvailable = fmt.Sprintf("%."+strconv.FormatInt(b.Scale, 10)+"f", float64(b.Available)/float64(divisor))
		}

		// Format on hold amount with scale
		formattedOnHold := strconv.FormatInt(b.OnHold, 10)

		if b.Scale > 0 {
			divisor := int64(1)

			for i := int64(0); i < b.Scale; i++ {
				divisor *= 10
			}

			formattedOnHold = fmt.Sprintf("%."+strconv.FormatInt(b.Scale, 10)+"f", float64(b.OnHold)/float64(divisor))
		}

		table.Append([]string{
			b.ID,
			b.AccountID,
			formattedAvailable,
			formattedOnHold,
			b.AssetCode,
			b.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	table.Render()

	if balances.Pagination != nil {
		output.Printf(f.factory.IOStreams.Out, "\nTotal: %d", len(balances.Items))

		if balances.Pagination.NextCursor != nil && *balances.Pagination.NextCursor != "" {
			output.Printf(f.factory.IOStreams.Out, ", Next page: mdz balance list --organization-id %s --ledger-id %s --cursor %s",
				f.OrganizationID, f.LedgerID, *balances.Pagination.NextCursor)
		}

		if balances.Pagination.PrevCursor != nil && *balances.Pagination.PrevCursor != "" {
			output.Printf(f.factory.IOStreams.Out, ", Previous page: mdz balance list --organization-id %s --ledger-id %s --cursor %s",
				f.OrganizationID, f.LedgerID, *balances.Pagination.PrevCursor)
		}
	}
}

func (f *factoryBalanceList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Limit the number of balances returned.")
	cmd.Flags().StringVar(&f.Cursor, "cursor", "", "Cursor for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "desc", "Sort order (asc or desc).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Filter by start date (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "Filter by end date (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryBalanceList {
	return &factoryBalanceList{
		factory:     f,
		repoBalance: rest.NewBalance(f),
		tuiInput:    tui.Input,
	}
}

func newCmdBalanceList(f *factoryBalanceList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists balances.",
		Long: utils.Format(
			"Lists all balances in the specified ledger. The results can be filtered",
			"and paginated using the available flags. Returns a table of balances",
			"or an error message.",
		),
		Example: utils.Format(
			"$ mdz balance list",
			"$ mdz balance list -h",
			"$ mdz balance list --organization-id <org-id> --ledger-id <ledger-id>",
			"$ mdz balance list --limit 20 --sort-order asc",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
