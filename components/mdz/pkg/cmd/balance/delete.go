package balance

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

type factoryBalanceDelete struct {
	factory     *factory.Factory
	repoBalance repository.Balance
	tuiInput    func(message string) (string, error)
	flagsDelete
}

type flagsDelete struct {
	OrganizationID string
	LedgerID       string
	BalanceID      string
}

func (f *factoryBalanceDelete) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("balance-id") && len(f.BalanceID) < 1 {
		id, err := f.tuiInput("Enter the balance-id")
		if err != nil {
			return errors.Wrap(err, "failed to get balance ID from input")
		}

		f.BalanceID = id
	}

	err := f.repoBalance.Delete(f.OrganizationID, f.LedgerID, f.BalanceID)
	if err != nil {
		return errors.CommandError("balance delete", err)
	}

	output.FormatAndPrint(f.factory, "The Balance has been successfully deleted.", "Balance", output.Deleted)

	return nil
}

func (f *factoryBalanceDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.BalanceID, "balance-id", "", "Specify the balance ID to delete.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryBalanceDelete {
	return &factoryBalanceDelete{
		factory:     f,
		repoBalance: rest.NewBalance(f),
		tuiInput:    tui.Input,
	}
}

func newCmdBalanceDelete(f *factoryBalanceDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes a balance.",
		Long: utils.Format(
			"Deletes a specific balance using its ID. This operation cannot be undone,",
			"and once deleted, the balance and its funds cannot be recovered.",
		),
		Example: utils.Format(
			"$ mdz balance delete",
			"$ mdz balance delete -h",
			"$ mdz balance delete --organization-id 123 --ledger-id 456 --balance-id 789",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
