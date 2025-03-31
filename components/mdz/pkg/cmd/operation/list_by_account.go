package operation

import (
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

type factoryOperationListByAccount struct {
	factory       *factory.Factory
	repoOperation repository.Operation
	tuiInput      func(message string) (string, error)
	flagsListByAccount
}

type flagsListByAccount struct {
	OrganizationID string
	LedgerID       string
	AccountID      string
	Limit          int
	Page           int
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryOperationListByAccount) runE(cmd *cobra.Command, _ []string) error {
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

	resp, err := f.repoOperation.GetByAccount(f.OrganizationID, f.LedgerID, f.AccountID, f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return err
	}

	f.printOperations(resp)

	return nil
}

func (f *factoryOperationListByAccount) printOperations(operations *mmodel.Operations) {
	if len(operations.Items) == 0 {
		output.Printf(f.factory.IOStreams.Out, "No operations found for account %s", f.AccountID)
		return
	}

	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"ID", "Transaction ID", "Type", "Amount", "Asset Code", "Created At"})

	for _, op := range operations.Items {
		table.Append([]string{
			op.ID,
			op.TransactionID,
			op.Type,
			strconv.FormatInt(op.Amount, 10),
			op.AssetCode,
			op.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	table.Render()

	if operations.Pagination != nil {
		output.Printf(f.factory.IOStreams.Out, "\nPage: %d, Total: %d", f.Page, len(operations.Items))

		if f.Page > 1 {
			output.Printf(f.factory.IOStreams.Out, ", Previous page: mdz operation list-by-account --organization-id %s --ledger-id %s --account-id %s --page %d",
				f.OrganizationID, f.LedgerID, f.AccountID, f.Page-1)
		}

		if len(operations.Items) == f.Limit {
			output.Printf(f.factory.IOStreams.Out, ", Next page: mdz operation list-by-account --organization-id %s --ledger-id %s --account-id %s --page %d",
				f.OrganizationID, f.LedgerID, f.AccountID, f.Page+1)
		}
	}
}

func (f *factoryOperationListByAccount) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify the account ID.")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Limit the number of operations returned.")
	cmd.Flags().IntVar(&f.Page, "page", 1, "Specify the page number for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "desc", "Sort order (asc or desc).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Filter by start date (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "Filter by end date (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacListByAccount(f *factory.Factory) *factoryOperationListByAccount {
	return &factoryOperationListByAccount{
		factory:       f,
		repoOperation: rest.NewOperation(f),
		tuiInput:      tui.Input,
	}
}

func newCmdOperationListByAccount(f *factoryOperationListByAccount) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-by-account",
		Short: "Lists operations for a specific account.",
		Long: utils.Format(
			"Lists all operations for a specific account in the specified ledger.",
			"The results can be filtered and paginated using the available flags.",
			"Returns a table of operations or an error message.",
		),
		Example: utils.Format(
			"$ mdz operation list-by-account",
			"$ mdz operation list-by-account -h",
			"$ mdz operation list-by-account --organization-id <org-id> --ledger-id <ledger-id> --account-id <account-id>",
			"$ mdz operation list-by-account --limit 20 --page 2 --sort-order asc",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
