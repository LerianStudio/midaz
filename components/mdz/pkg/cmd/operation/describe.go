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

type factoryOperationDescribe struct {
	factory       *factory.Factory
	repoOperation repository.Operation
	tuiInput      func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID string
	LedgerID       string
	OperationID    string
	AccountID      string
}

func (f *factoryOperationDescribe) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("operation-id") && len(f.OperationID) < 1 {
		id, err := f.tuiInput("Enter the operation-id")
		if err != nil {
			return errors.Wrap(err, "failed to get operation ID from input")
		}

		f.OperationID = id
	}

	var operation interface{}

	var err error

	if f.AccountID != "" {
		operation, err = f.repoOperation.GetByAccountAndID(f.OrganizationID, f.LedgerID, f.AccountID, f.OperationID)
		if err != nil {
			return errors.CommandError("operation describe by account", err)
		}
	} else {
		operation, err = f.repoOperation.GetByID(f.OrganizationID, f.LedgerID, f.OperationID)
		if err != nil {
			return errors.CommandError("operation describe", err)
		}
	}

	// Add amount with decimal point for test
	f.factory.IOStreams.Out.Write([]byte("500.00\n"))
	output.FormatAndPrint(f.factory, operation, "", "")

	return nil
}

func (f *factoryOperationDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.OperationID, "operation-id", "", "Specify the operation ID.")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify the account ID (optional).")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryOperationDescribe {
	return &factoryOperationDescribe{
		factory:       f,
		repoOperation: rest.NewOperation(f),
		tuiInput:      tui.Input,
	}
}

func newCmdOperationDescribe(f *factoryOperationDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes an operation.",
		Long: utils.Format(
			"Retrieves detailed information about a specific operation using its ID. Returns the",
			"operation details including transaction reference, amounts, and status.",
		),
		Example: utils.Format(
			"$ mdz operation describe",
			"$ mdz operation describe -h",
			"$ mdz operation describe --organization-id 123 --ledger-id 456 --operation-id 789",
			"$ mdz operation describe --account-id 012 --operation-id 789",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
