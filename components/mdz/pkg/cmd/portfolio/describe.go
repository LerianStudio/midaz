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

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryPortfolioDescribe struct {
	factory        *factory.Factory
	repoPortfolio  repository.Portfolio
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	PortfolioID    string
	Out            string
	JSON           bool
}

func (f *factoryPortfolioDescribe) ensureFlagInput(cmd *cobra.Command) error {
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

func (f *factoryPortfolioDescribe) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	asset, err := f.repoPortfolio.GetByID(f.OrganizationID, f.LedgerID, f.PortfolioID)

	if err != nil {
		return err
	}

	return f.outputPortfolio(cmd, asset)
}

func (f *factoryPortfolioDescribe) outputPortfolio(cmd *cobra.Command, asset *mmodel.Portfolio) error {
	if f.JSON || cmd.Flags().Changed("out") {
		b, err := json.Marshal(asset)

		if err != nil {
			return err
		}

		if cmd.Flags().Changed("out") {
			if len(f.Out) == 0 {
				return errors.New("the file path was not entered")
			}

			err = utils.WriteDetailsToFile(b, f.Out)
			if err != nil {
				return errors.New("failed when trying to write the output file " + err.Error())
			}

			output.Printf(f.factory.IOStreams.Out, "File successfully written to: "+f.Out)

			return nil
		}

		output.Printf(f.factory.IOStreams.Out, string(b))

		return nil
	}

	f.describePrint(asset)

	return nil
}

func (f *factoryPortfolioDescribe) describePrint(asset *mmodel.Portfolio) {
	tbl := table.New("FIELDS", "VALUES")

	if !f.factory.NoColor {
		headerFmt := color.New(color.FgYellow).SprintfFunc()
		fieldFmt := color.New(color.FgYellow).SprintfFunc()
		tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	}

	tbl.WithWriter(f.factory.IOStreams.Out)

	tbl.AddRow("ID:", asset.ID)
	tbl.AddRow("Name:", asset.Name)
	tbl.AddRow("Entity ID:", asset.EntityID)
	tbl.AddRow("Status Code:", asset.Status.Code)

	if asset.Status.Description != nil {
		tbl.AddRow("Status Description:", *asset.Status.Description)
	}

	tbl.AddRow("Organization ID:", asset.OrganizationID)
	tbl.AddRow("Ledger ID:", asset.LedgerID)
	tbl.AddRow("Created At:", asset.CreatedAt)
	tbl.AddRow("Update At:", asset.UpdatedAt)

	if asset.DeletedAt != nil {
		tbl.AddRow("Delete At:", *asset.DeletedAt)
	}

	tbl.AddRow("Metadata:", asset.Metadata)

	tbl.Print()
}

func (f *factoryPortfolioDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Out, "out", "", "Exports the output to the given <file_path/file_name.ext>")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID to retrieve details")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryPortfolioDescribe {
	return &factoryPortfolioDescribe{
		factory:       f,
		repoPortfolio: rest.NewPortfolio(f),
		tuiInput:      tui.Input,
	}
}

func newCmdPortfolioDescribe(f *factoryPortfolioDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Shows details of a specific portfolio.",
		Long: utils.Format(
			"Provides a detailed view of a selected portfolio, including",
			"information on accounts, assets, entities and hierarchical",
			"relationships, such as sub-accounts and parent entities, to",
			"better understand its structure.",
		),
		Example: utils.Format(
			"$ mdz portfolio describe --organization-id 12341234 --ledger-id 12312 --portfolio-id 432123",
			"$ mdz portfolio describe",
			"$ mdz portfolio describe -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
