package account

import (
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/v3/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryAccountUpdate struct {
	factory     *factory.Factory
	repoAccount repository.Account
	tuiInput    func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	PortfolioID       string
	AccountID         string
	Name              string
	StatusCode        string
	StatusDescription string
	Alias             string
	SegmentID         string
	Metadata          string
	JSONFile          string
}

func (f *factoryAccountUpdate) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("account-id") && len(f.AccountID) < 1 {
		id, err := f.tuiInput("Enter your account-id")
		if err != nil {
			return err
		}

		f.AccountID = id
	}

	return nil
}

func (f *factoryAccountUpdate) runE(cmd *cobra.Command, _ []string) error {
	account := mmodel.UpdateAccountInput{}

	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &account)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&account)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoAccount.Update(f.OrganizationID, f.LedgerID, f.AccountID, account)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Account", output.Updated)

	return nil
}

func (f *factoryAccountUpdate) UpdateRequestFromFlags(account *mmodel.UpdateAccountInput) error {
	account.Name = f.Name
	account.Status.Code = f.StatusCode

	if len(f.StatusDescription) > 0 {
		account.Status.Description = &f.StatusDescription
	}

	if len(f.SegmentID) > 0 {
		account.SegmentID = &f.SegmentID
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	account.Metadata = metadata

	return nil
}

func (f *factoryAccountUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID.")
	cmd.Flags().StringVar(&f.AccountID, "account-id", "", "Specify the account ID.")
	cmd.Flags().StringVar(&f.Name, "name", "", "Legal name of the Account.")
	cmd.Flags().StringVar(&f.Alias, "alias", "", "Set an alias for the ledger.")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")
	cmd.Flags().StringVar(&f.SegmentID, "segment-id", "", "Specify the segment ID.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being Updated; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryAccountUpdate {
	return &factoryAccountUpdate{
		factory:     f,
		repoAccount: rest.NewAccount(f),
		tuiInput:    tui.Input,
	}
}

func newCmdAccountUpdate(f *factoryAccountUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates an account.",
		Long: utils.Format(
			"Modifies the data of an existing account in the portfolio.",
			"Returns a success or error message depending on the existence",
			"and update of the account.",
		),
		Example: utils.Format(
			"$ mdz account update",
			"$ mdz account update -h",
			"$ mdz account update --json-file payload.json",
			"$ cat payload.json | mdz account update --organization-id '1234' --ledger-id '4421' --portfolio-id '99984' --account-id '45232' --json-file -",
			"$ mdz account update --organization-id '1234' --ledger-id '4421' --portfolio-id '99984' --account-id '55232' --name 'Gislason LLCT'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
