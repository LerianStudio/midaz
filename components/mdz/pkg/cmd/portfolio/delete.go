package portfolio

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryPortfolioDelete struct {
	factory        *factory.Factory
	repoPortfolio  repository.Portfolio
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	PortfolioID    string
}

func (f *factoryPortfolioDelete) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("portfolio-id") && len(f.PortfolioID) < 1 {
		id, err := f.tuiInput("Enter your portfolio-id")
		if err != nil {
			return err
		}

		f.PortfolioID = id
	}

	return nil
}

func (f *factoryPortfolioDelete) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	err := f.repoPortfolio.Delete(f.OrganizationID, f.LedgerID, f.PortfolioID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.PortfolioID, "Portfolio", output.Deleted)

	return nil
}

func (f *factoryPortfolioDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryPortfolioDelete {
	return &factoryPortfolioDelete{
		factory:       f,
		repoPortfolio: rest.NewPortfolio(f),
		tuiInput:      tui.Input,
	}
}

func newCmdPortfolioDelete(f *factoryPortfolioDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Removes a portfolio from the system.",
		Long: utils.Format(
			"Deletes a specific portfolio, including all associated accounts and",
			"sub-accounts, ensuring that the portfolio data is securely removed",
			"from the system.",
		),
		Example: utils.Format(
			"$ mdz portfolio delete --organization-id '1234' --ledger-id '4421' --portfolio-id '55232'",
			"$ mdz portfolio delete -i 12314",
			"$ mdz portfolio delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
