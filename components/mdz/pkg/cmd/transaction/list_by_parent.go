package transaction

import (
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	rest "github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type listByParentOptions struct {
	organizationID string
	ledgerID       string
	parentID       string
	limit          int
	page           int
	sortOrder      string
	startDate      string
	endDate        string
	dryRun         bool
	factory        rest.Factory
}

func (o *listByParentOptions) initFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.organizationID, "organization-id", "o", "", "Organization ID")
	cmd.Flags().StringVarP(&o.ledgerID, "ledger-id", "l", "", "Ledger ID")
	cmd.Flags().StringVarP(&o.parentID, "parent-id", "p", "", "Parent transaction ID")
	cmd.Flags().IntVarP(&o.limit, "limit", "", 20, "Maximum number of transactions to return")
	cmd.Flags().IntVarP(&o.page, "page", "", 1, "Page number for pagination")
	cmd.Flags().StringVarP(&o.sortOrder, "sort", "", "desc", "Sort order (asc or desc)")
	cmd.Flags().StringVarP(&o.startDate, "start-date", "", "", "Filter transactions after this date (ISO format)")
	cmd.Flags().StringVarP(&o.endDate, "end-date", "", "", "Filter transactions before this date (ISO format)")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "d", false, "Only print the command that would be executed")

	_ = cmd.MarkFlagRequired("organization-id")
	_ = cmd.MarkFlagRequired("ledger-id")
	_ = cmd.MarkFlagRequired("parent-id")
}

func (o *listByParentOptions) validateOptions() error {
	if o.organizationID == "" {
		return fmt.Errorf("organization-id is required")
	}

	if o.ledgerID == "" {
		return fmt.Errorf("ledger-id is required")
	}

	if o.parentID == "" {
		return fmt.Errorf("parent-id is required")
	}

	if o.limit < 1 {
		return fmt.Errorf("limit must be greater than 0")
	}

	if o.page < 1 {
		return fmt.Errorf("page must be greater than 0")
	}

	if o.sortOrder != "asc" && o.sortOrder != "desc" {
		return fmt.Errorf("sort must be either 'asc' or 'desc'")
	}

	// Validate date formats if provided
	if o.startDate != "" {
		if err := utils.ValidateDate(o.startDate); err != nil {
			return fmt.Errorf("invalid start-date format: %v", err)
		}
	}

	if o.endDate != "" {
		if err := utils.ValidateDate(o.endDate); err != nil {
			return fmt.Errorf("invalid end-date format: %v", err)
		}
	}

	// Validate UUIDs
	if _, err := uuid.Parse(o.organizationID); err != nil {
		return fmt.Errorf("invalid organization ID format: %v", err)
	}

	if _, err := uuid.Parse(o.ledgerID); err != nil {
		return fmt.Errorf("invalid ledger ID format: %v", err)
	}

	if _, err := uuid.Parse(o.parentID); err != nil {
		return fmt.Errorf("invalid parent ID format: %v", err)
	}

	return nil
}

func (o *listByParentOptions) run() (*mmodel.Transactions, error) {
	if err := o.validateOptions(); err != nil {
		return nil, err
	}

	if o.dryRun {
		fmt.Printf("Would list transactions with parent ID %s\n", o.parentID)
		fmt.Printf("Organization ID: %s\n", o.organizationID)
		fmt.Printf("Ledger ID: %s\n", o.ledgerID)
		fmt.Printf("Limit: %d, Page: %d, Sort: %s\n", o.limit, o.page, o.sortOrder)
		if o.startDate != "" {
			fmt.Printf("Start Date: %s\n", o.startDate)
		}
		if o.endDate != "" {
			fmt.Printf("End Date: %s\n", o.endDate)
		}
		return nil, nil
	}

	// Use the GetByParentIDPaginated method directly
	txn := o.factory.Transaction()
	
	// Check if the GetByParentIDPaginated method exists on the transaction object
	if txnWithPagination, ok := interface{}(txn).(interface {
		GetByParentIDPaginated(string, string, string, int, int, string, string, string) (*mmodel.Transactions, error)
	}); ok {
		return txnWithPagination.GetByParentIDPaginated(
			o.organizationID,
			o.ledgerID,
			o.parentID,
			o.limit,
			o.page,
			o.sortOrder,
			o.startDate,
			o.endDate,
		)
	}
	
	// Fallback to getting a single transaction if the paginated method is not available
	singleTxn, err := txn.GetByParentID(o.organizationID, o.ledgerID, o.parentID)
	if err != nil {
		return nil, err
	}
	
	// Convert single transaction to transactions list
	return &mmodel.Transactions{
		Items: []mmodel.Transaction{*singleTxn},
		Page:  1,
		Limit: 1,
	}, nil
}

type injectFacListByParent struct {
	factory *factory.Factory
}

func (i *injectFacListByParent) injectFactory() rest.Factory {
	return rest.NewFactory(i.factory)
}

func newInjectFacListByParent(factory *factory.Factory) *injectFacListByParent {
	return &injectFacListByParent{
		factory: factory,
	}
}

func newCmdTransactionListByParent(i *injectFacListByParent) *cobra.Command {
	o := &listByParentOptions{
		factory: i.injectFactory(),
	}

	cmd := &cobra.Command{
		Use:   "list-by-parent",
		Short: "List child transactions of a parent transaction",
		Long: utils.Format(
			"The list-by-parent command retrieves all child transactions associated with",
			"a specific parent transaction ID. This is useful for tracking transaction",
			"relationships such as refunds, adjustments, or split transactions.",
		),
		Example: utils.Format(
			"$ mdz transaction list-by-parent --organization-id <id> --ledger-id <id> --parent-id <id>",
			"$ mdz transaction list-by-parent -o <id> -l <id> -p <id> --limit 50 --page 2",
			"$ mdz transaction list-by-parent -o <id> -l <id> -p <id> --start-date 2023-01-01 --end-date 2023-12-31",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			txns, err := o.run()
			if err != nil {
				return err
			}

			if o.dryRun {
				return nil
			}

			// Convert to API transaction model for display
			if txns == nil || len(txns.Items) == 0 {
				fmt.Println("No child transactions found for parent ID:", o.parentID)
				return nil
			}

			// Display transactions in tabular format
			fmt.Printf("Child transactions for parent ID: %s\n\n", o.parentID)

			return utils.PrintTable(
				[]string{"ID", "Description", "Status", "Type", "Created At"},
				func() [][]string {
					var data [][]string
					for _, txn := range txns.Items {
						apiTxn := model.AsTransaction(&txn)
						data = append(data, []string{
							apiTxn.ID,
							utils.TruncateString(apiTxn.Description, 30),
							apiTxn.Status.Code,
							apiTxn.Type,
							apiTxn.CreatedAt.Format("2006-01-02 15:04:05"),
						})
					}
					return data
				}(),
			)
		},
	}

	o.initFlags(cmd)
	return cmd
}