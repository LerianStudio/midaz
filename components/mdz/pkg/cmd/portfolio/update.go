package portfolio

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

type factoryPortfolioUpdate struct {
	factory       *factory.Factory
	repoPortfolio repository.Portfolio
	tuiInput      func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	PortfolioID       string
	Name              string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factoryPortfolioUpdate) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("portfolio-id") && len(f.PortfolioID) < 1 {
		id, err := tui.Input("Enter your portfolio-id")
		if err != nil {
			return err
		}

		f.PortfolioID = id
	}

	return nil
}

func (f *factoryPortfolioUpdate) runE(cmd *cobra.Command, _ []string) error {
	portfolio := mmodel.UpdatePortfolioInput{}

	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &portfolio)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&portfolio)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoPortfolio.Update(f.OrganizationID, f.LedgerID, f.PortfolioID, portfolio)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Portfolio", output.Updated)

	return nil
}

func (f *factoryPortfolioUpdate) UpdateRequestFromFlags(portfolio *mmodel.UpdatePortfolioInput) error {
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

func (f *factoryPortfolioUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID to retrieve details")
	cmd.Flags().StringVar(&f.Name, "name", "", "Legal name of the Portfolio.")
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

func newInjectFacUpdate(f *factory.Factory) *factoryPortfolioUpdate {
	return &factoryPortfolioUpdate{
		factory:       f,
		repoPortfolio: rest.NewPortfolio(f),
		tuiInput:      tui.Input,
	}
}

func newCmdPortfolioUpdate(f *factoryPortfolioUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates information on an existing portfolio.",
		Long: utils.Format(
			"Allows you to modify the details of a specific portfolio, including",
			"adjustments to accounts, assets, and information related to parent",
			"and child entities, to ensure that the portfolio data is always up to date.",
		),
		Example: utils.Format(
			"$ mdz portfolio update",
			"$ mdz portfolio update -h",
			"$ mdz portfolio update --json-file payload.json",
			"$ cat payload.json | mdz portfolio update --organization-id '1234' --ledger-id '4421' --portfolio-id '45232' --json-file -",
			"$ mdz portfolio update --organization-id '1234' --ledger-id '4421' --portfolio-id '55232' --name 'Gislason LLCT'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
