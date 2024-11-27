package account

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryAccountDelete struct {
	factory        *factory.Factory
	repoAccount    repository.Account
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	PortfolioID    string
	AccountID      string
}

func (f *factoryAccountDelete) ensureFlagInput(cmd *cobra.Command) error {
	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := tui.Input("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.LedgerID = id
	}

	if !cmd.Flags().Changed("potfolio-id") && len(f.PortfolioID) < 1 {
		id, err := tui.Input("Enter your portfolio-id")
		if err != nil {
			return err
		}

		f.PortfolioID = id
	}

	if !cmd.Flags().Changed("account-id") && len(f.AccountID) < 1 {
		id, err := tui.Input("Enter your account-id")
		if err != nil {
			return err
		}

		f.AccountID = id
	}

	return nil
}

func (f *factoryAccountDelete) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	err := f.repoAccount.Delete(f.OrganizationID, f.LedgerID, f.PortfolioID, f.AccountID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.AccountID, "Account", output.Deleted)

	return nil
}

func (f *factoryAccountDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify the account ID")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryAccountDelete {
	return &factoryAccountDelete{
		factory:     f,
		repoAccount: rest.NewAccount(f),
		tuiInput:    tui.Input,
	}
}

func newCmdAccountDelete(f *factoryAccountDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes an account.",
		Long: utils.Format(
			"Removes a specific account from the portfolio, using its ID as a ",
			"parameter. Returns a success or error message, depending on the",
			"existence of the account.",
		),
		Example: utils.Format(
			"$ mdz account delete --organization-id '1234' --ledger-id '4421' --portfolio-id '743' --account-id '55232'",
			"$ mdz account delete -i 12314",
			"$ mdz account delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
