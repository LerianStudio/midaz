package product

import (
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryProductUpdate struct {
	factory     *factory.Factory
	repoProduct repository.Product
	tuiInput    func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	ProductID         string
	Name              string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factoryProductUpdate) ensureFlagInput(cmd *cobra.Command) error {
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

func (f *factoryProductUpdate) runE(cmd *cobra.Command, _ []string) error {
	product := mmodel.UpdateProductInput{}

	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &product)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&product)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoProduct.Update(f.OrganizationID, f.LedgerID, f.ProductID, product)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Product", output.Updated)

	return nil
}

func (f *factoryProductUpdate) UpdateRequestFromFlags(portfolio *mmodel.UpdateProductInput) error {
	portfolio.Name = f.Name
	portfolio.Status.Code = f.StatusCode

	if len(f.StatusDescription) > 0 {
		portfolio.Status.Description = &f.StatusDescription
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	portfolio.Metadata = metadata

	return nil
}

func (f *factoryProductUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.ProductID, "product-id", "", "Specify the portfolio ID")
	cmd.Flags().StringVar(&f.Name, "name", "", "Legal name of the Product.")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")

	// Flags command Update
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being Updated; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryProductUpdate {
	return &factoryProductUpdate{
		factory:     f,
		repoProduct: rest.NewProduct(f),
		tuiInput:    tui.Input,
	}
}

func newCmdProductUpdate(f *factoryProductUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates an existing product with new policies.",
		Long: utils.Format(
			"The update subcommand allows you to adjust the policies and settings",
			"of an existing product. With it, you can modify the clustering rules,",
			"adapting the grouping of clients according to changes in business",
			"strategies and needs.",
		),
		Example: utils.Format(
			"$ mdz product update",
			"$ mdz product update -h",
			"$ mdz product update --json-file payload.json",
			"$ cat payload.json | mdz product update --organization-id '1234' --ledger-id '4421' --product-id '45232' --json-file -",
			"$ mdz portfolio update --organization-id '1234' --ledger-id '4421' --portfolio-id '55232' --name 'Gislason LLCT'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
