package operation

import (
	"fmt"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryOperationList struct {
	factory      *factory.Factory
	repoOperation repository.Operation
	tuiInput     func(message string) (string, error)
	flagsList
}

type flagsList struct {
	OrganizationID string
	LedgerID       string
	Limit          int
	Page           int
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryOperationList) runE(cmd *cobra.Command, _ []string) error {
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

	resp, err := f.repoOperation.Get(f.OrganizationID, f.LedgerID, f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
	if err != nil {
		return err
	}

	f.printOperations(resp)

	return nil
}

func (f *factoryOperationList) printOperations(operations *mmodel.Operations) {
	if len(operations.Items) == 0 {
		output.Printf(f.factory.IOStreams.Out, "No operations found")
		return
	}

	table := output.NewTable(f.factory.IOStreams.Out)
	table.SetHeader([]string{"ID", "Transaction ID", "Account ID", "Type", "Amount", "Asset Code", "Created At"})

	for _, op := range operations.Items {
		table.Append([]string{
			op.ID,
			op.TransactionID,
			op.AccountID,
			op.Type,
			fmt.Sprintf("%d", op.Amount),
			op.AssetCode,
			op.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	table.Render()

	if operations.Pagination != nil {
		output.Printf(f.factory.IOStreams.Out, "\nPage: %d, Total: %d", f.Page, len(operations.Items))
		if f.Page > 1 {
			output.Printf(f.factory.IOStreams.Out, ", Previous page: mdz operation list --organization-id %s --ledger-id %s --page %d", 
				f.OrganizationID, f.LedgerID, f.Page-1)
		}
		if len(operations.Items) == f.Limit {
			output.Printf(f.factory.IOStreams.Out, ", Next page: mdz operation list --organization-id %s --ledger-id %s --page %d", 
				f.OrganizationID, f.LedgerID, f.Page+1)
		}
	}
}

func (f *factoryOperationList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Limit the number of operations returned.")
	cmd.Flags().IntVar(&f.Page, "page", 1, "Specify the page number for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "desc", "Sort order (asc or desc).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Filter by start date (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "Filter by end date (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryOperationList {
	return &factoryOperationList{
		factory:      f,
		repoOperation: rest.NewOperation(f),
		tuiInput:     tui.Input,
	}
}

func newCmdOperationList(f *factoryOperationList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists operations.",
		Long: utils.Format(
			"Lists all operations in the specified ledger. The results can be filtered",
			"and paginated using the available flags. Returns a table of operations",
			"or an error message.",
		),
		Example: utils.Format(
			"$ mdz operation list",
			"$ mdz operation list -h",
			"$ mdz operation list --organization-id <org-id> --ledger-id <ledger-id>",
			"$ mdz operation list --limit 20 --page 2 --sort-order asc",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
