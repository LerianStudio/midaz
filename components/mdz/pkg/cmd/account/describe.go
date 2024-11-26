package account

import (
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryAccountDescribe struct {
	factory        *factory.Factory
	repoAccount    repository.Account
	OrganizationID string
	LedgerID       string
	PortfolioID    string
	AccountID      string
	Out            string
	JSON           bool
}

func (f *factoryAccountDescribe) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("account-id") && len(f.AccountID) < 1 {
		id, err := tui.Input("Enter your account-id")
		if err != nil {
			return err
		}

		f.AccountID = id
	}

	return nil
}

func (f *factoryAccountDescribe) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	account, err := f.repoAccount.GetByID(f.OrganizationID, f.LedgerID, f.PortfolioID, f.AccountID)
	if err != nil {
		return err
	}

	return f.outputAccount(cmd, account)
}

func (f *factoryAccountDescribe) outputAccount(cmd *cobra.Command, account *mmodel.Account) error {
	if f.JSON || cmd.Flags().Changed("out") {
		b, err := json.Marshal(account)
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

	f.describePrint(account)

	return nil
}

func (f *factoryAccountDescribe) describePrint(account *mmodel.Account) {
	tbl := table.New("FIELDS", "VALUES")

	headerFmt := color.New(color.FgYellow).SprintfFunc()
	fieldFmt := color.New(color.FgYellow).SprintfFunc()

	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	tbl.WithWriter(f.factory.IOStreams.Out)

	tbl.AddRow("ID:", account.ID)
	tbl.AddRow("Asset Code:", account.AssetCode)
	tbl.AddRow("Name:", account.Name)

	if account.EntityID != nil {
		tbl.AddRow("Entity ID:", *account.EntityID)
	}

	if account.ProductID != nil {
		tbl.AddRow("Product ID:", *account.ProductID)
	}

	if account.ParentAccountID != nil {
		tbl.AddRow("Parent Account ID:", *account.ParentAccountID)
	}

	if account.Alias != nil {
		tbl.AddRow("Alias:", *account.Alias)
	}

	tbl.AddRow("Type:", account.Type)
	tbl.AddRow("Status Code:", account.Status.Code)

	if account.Status.Description != nil {
		tbl.AddRow("Status Description:", *account.Status.Description)
	}

	tbl.AddRow("Organization ID:", account.OrganizationID)
	tbl.AddRow("Ledger ID:", account.LedgerID)
	tbl.AddRow("Created At:", account.CreatedAt)
	tbl.AddRow("Update At:", account.UpdatedAt)

	if account.DeletedAt != nil {
		tbl.AddRow("Delete At:", *account.DeletedAt)
	}

	tbl.AddRow("Metadata:", account.Metadata)

	tbl.Print()
}

func (f *factoryAccountDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Out, "out", "", "Exports the output to the given <file_path/file_name.ext>")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID.")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify the account ID to details.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryAccountDescribe {
	return &factoryAccountDescribe{
		factory:     f,
		repoAccount: rest.NewAccount(f),
	}
}

func newCmdAccountDescribe(f *factoryAccountDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Displays details of an account.",
		Long: utils.Format(
			"Displays detailed information about a specific account, using its",
			"ID as a parameter. Returns an error message if the account is not found.",
		),
		Example: utils.Format(
			"$ mdz account describe --organization-id 12341234 --ledger-id 12312 --portfolio-id 432123",
			"$ mdz account describe",
			"$ mdz account describe -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
