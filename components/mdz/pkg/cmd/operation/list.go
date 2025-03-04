package operation

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryOperationList struct {
	factory       *factory.Factory
	repoOperation repository.Operation
	tuiInput      func(message string) (string, error)
	flagsListAll
}

type flagsListAll struct {
	OrganizationID string
	LedgerID       string
	AccountID      string
	Page           int
	Limit          int
	SortOrder      string
	StartDate      string
	EndDate        string
}

func (f *factoryOperationList) runE(cmd *cobra.Command, _ []string) error {
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

	var operations interface{}
	var err error

	if f.AccountID != "" {
		operations, err = f.repoOperation.GetByAccount(
			f.OrganizationID, f.LedgerID, f.AccountID, f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
		if err != nil {
			return errors.CommandError("operation list by account", err)
		}
	} else {
		operations, err = f.repoOperation.Get(
			f.OrganizationID, f.LedgerID, f.Limit, f.Page, f.SortOrder, f.StartDate, f.EndDate)
		if err != nil {
			return errors.CommandError("operation list", err)
		}
	}

	output.FormatAndPrint(f.factory, operations, "", "")

	return nil
}

func (f *factoryOperationList) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify an account ID to filter operations by account.")
	cmd.Flags().IntVar(&f.Page, "page", 1, "Page number for pagination.")
	cmd.Flags().IntVar(&f.Limit, "limit", 10, "Number of items per page for pagination.")
	cmd.Flags().StringVar(&f.SortOrder, "sort-order", "", "Sorting order for results (ASC or DESC).")
	cmd.Flags().StringVar(&f.StartDate, "start-date", "", "Start date filter (format: YYYY-MM-DD).")
	cmd.Flags().StringVar(&f.EndDate, "end-date", "", "End date filter (format: YYYY-MM-DD).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacList(f *factory.Factory) *factoryOperationList {
	return &factoryOperationList{
		factory:       f,
		repoOperation: rest.NewOperation(f),
		tuiInput:      tui.Input,
	}
}

func newCmdOperationList(f *factoryOperationList) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists operations.",
		Long: utils.Format(
			"Lists operations in the specified ledger. Can be filtered by account ID. Returns a",
			"list of operations with pagination support.",
		),
		Example: utils.Format(
			"$ mdz operation list",
			"$ mdz operation list -h",
			"$ mdz operation list --organization-id 123 --ledger-id 456",
			"$ mdz operation list --account-id 789 --limit 20 --page 2",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
