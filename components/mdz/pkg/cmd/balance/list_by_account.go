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

type factoryBalanceListByAccount struct {
	factory     *factory.Factory
	repoBalance repository.Balance
	tuiInput    func(message string) (string, error)
	flagsListByAccount
}

type flagsListByAccount struct {
	OrganizationID string
	LedgerID       string
	AccountID      string
	Limit          int
	Cursor         string
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryBalanceListByAccount) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("account-id") && len(f.AccountID) < 1 {
		id, err := f.tuiInput("Enter your account-id")
		if err != nil {
			return err
		}

		f.AccountID = id
	}

	resp, err := f.repoBalance.GetByAccount(f.OrganizationID, f.LedgerID, f.AccountID, f.Limit, f.Cursor, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return err
	}

	f.printBalances(resp)

	return nil
}

func (f *factoryBalanceListByAccount) printBalances(balances *mmodel.Balances) {
	if len(balances.Items) == 0 {
		output.Printf(f.factory.IOStreams.Out, "No balances found for account %s", f.AccountID)
		return
	}

	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"ID", "Available", "On Hold", "Asset Code", "Created At"})

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
			output.Printf(f.factory.IOStreams.Out, ", Next page: mdz balance list-by-account --organization-id %s --ledger-id %s --account-id %s --cursor %s",
				f.OrganizationID, f.LedgerID, f.AccountID, *balances.Pagination.NextCursor)
		}

		if balances.Pagination.PrevCursor != nil && *balances.Pagination.PrevCursor != "" {
			output.Printf(f.factory.IOStreams.Out, ", Previous page: mdz balance list-by-account --organization-id %s --ledger-id %s --account-id %s --cursor %s",
				f.OrganizationID, f.LedgerID, f.AccountID, *balances.Pagination.PrevCursor)
		}
	}
}

func (f *factoryBalanceListByAccount) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify the account ID.")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Limit the number of balances returned.")
	cmd.Flags().StringVar(&f.Cursor, "cursor", "", "Cursor for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "desc", "Sort order (asc or desc).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Filter by start date (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "Filter by end date (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacListByAccount(f *factory.Factory) *factoryBalanceListByAccount {
	return &factoryBalanceListByAccount{
		factory:     f,
		repoBalance: rest.NewBalance(f),
		tuiInput:    tui.Input,
	}
}

func newCmdBalanceListByAccount(f *factoryBalanceListByAccount) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-by-account",
		Short: "Lists balances for a specific account.",
		Long: utils.Format(
			"Lists all balances for a specific account in the specified ledger.",
			"The results can be filtered and paginated using the available flags.",
			"Returns a table of balances or an error message.",
		),
		Example: utils.Format(
			"$ mdz balance list-by-account",
			"$ mdz balance list-by-account -h",
			"$ mdz balance list-by-account --organization-id <org-id> --ledger-id <ledger-id> --account-id <account-id>",
			"$ mdz balance list-by-account --limit 20 --sort-order asc",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
