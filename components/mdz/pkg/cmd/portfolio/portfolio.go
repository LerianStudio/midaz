// Package portfolio provides CLI commands for managing portfolios.
//
// This package implements the "mdz portfolio" command group with subcommands
// for create, list, describe, update, and delete operations.
package portfolio

import (
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

// factoryPortfolio wraps the factory for portfolio commands.
type factoryPortfolio struct {
	factory *factory.Factory
}

// setCmds registers all portfolio subcommands.
func (f *factoryPortfolio) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdPortfolioCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdPortfolioList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdPortfolioDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdPortfolioUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdPortfolioDelete(newInjectFacDelete(f.factory)))
}

// NewCmdPortfolio creates the "portfolio" command with all subcommands.
//
// Returns a Cobra command configured with create, list, describe, update, and delete subcommands.
func NewCmdPortfolio(f *factory.Factory) *cobra.Command {
	fOrg := factoryPortfolio{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "portfolio",
		Short: "Manages groups of accounts in a portfolio.",
		Long: utils.Format(
			"The portfolio command organizes and manages groups of accounts, ",
			"allowing you to manage a variety of assets. Just like a physical ",
			"portfolio, the portfolio contains accounts for different assets,",
			"offering a consolidated view and allowing actions such as creating,",
			"updating, listing, describing and deleting accounts and sub-accounts",
			"associated with an entity.",
		),
		Example: utils.Format(
			"$ mdz portfolio",
			"$ mdz portfolio -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
