package product

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryProductDelete struct {
	factory        *factory.Factory
	repoProduct    repository.Product
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	ProductID      string
}

func (f *factoryProductDelete) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("product-id") && len(f.ProductID) < 1 {
		id, err := tui.Input("Enter your product-id")
		if err != nil {
			return err
		}

		f.ProductID = id
	}

	return nil
}

func (f *factoryProductDelete) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	err := f.repoProduct.Delete(f.OrganizationID, f.LedgerID, f.ProductID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.ProductID, "Product", output.Deleted)

	return nil
}

func (f *factoryProductDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.ProductID, "product-id", "", "Specify the portfolio ID")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryProductDelete {
	return &factoryProductDelete{
		factory:     f,
		repoProduct: rest.NewProduct(f),
		tuiInput:    tui.Input,
	}
}

func newCmdProductDelete(f *factoryProductDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Removes an existing product.",
		Long: utils.Format(
			"The delete subcommand allows you to delete a product, removing its",
			"settings and clustering rules. It is useful for deactivating obsolete",
			"clusters or adjusting the organization of products without changing",
			"the structure of customers.",
		),
		Example: utils.Format(
			"$ mdz product delete --organization-id '1234' --ledger-id '4421' --product-id '55232'",
			"$ mdz product delete -i 12314",
			"$ mdz product delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
