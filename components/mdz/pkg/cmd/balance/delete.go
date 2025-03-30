package balance

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryBalanceDelete struct {
	factory    *factory.Factory
	repoBalance repository.Balance
	tuiInput   func(message string) (string, error)
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

	if !cmd.Flags().Changed("balance-id") && len(f.BalanceID) < 1 {
		id, err := f.tuiInput("Enter your balance-id")
		if err != nil {
			return err
		}

		f.BalanceID = id
	}

	if err := f.repoBalance.Delete(f.OrganizationID, f.LedgerID, f.BalanceID); err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.BalanceID, "Balance", output.Deleted)

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
		factory:    f,
		repoBalance: rest.NewBalance(f),
		tuiInput:   tui.Input,
	}
}

func newCmdBalanceDelete(f *factoryBalanceDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes a balance.",
		Long: utils.Format(
			"Deletes a specific balance from the ledger. This operation is permanent",
			"and cannot be undone. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz balance delete",
			"$ mdz balance delete -h",
			"$ mdz balance delete --organization-id <org-id> --ledger-id <ledger-id> --balance-id <balance-id>",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
