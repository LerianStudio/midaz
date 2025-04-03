package account

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

type factoryAccountCreate struct {
	factory     *factory.Factory
	repoAccount repository.Account
	tuiInput    func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID    string
	LedgerID          string
	PortfolioID       string
	AssetCode         string
	Name              string
	Alias             string
	Type              string
	ParentAccountID   string
	SegmentID         string
	EntityID          string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factoryAccountCreate) runE(cmd *cobra.Command, _ []string) error {
	account := mmodel.CreateAccountInput{}

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

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &account)

		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&account)

		if err != nil {
			return err
		}
	}

	resp, err := f.repoAccount.Create(f.OrganizationID, f.LedgerID, account)

	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Account", output.Created)

	return nil
}

func (f *factoryAccountCreate) createRequestFromFlags(account *mmodel.CreateAccountInput) error {
	var err error

	account.AssetCode = f.AssetCode

	account.Name, err = utils.AssignStringField(f.Name, "name", f.tuiInput)
	if err != nil {
		return err
	}

	if len(f.Alias) > 0 {
		account.Alias = &f.Alias
	}

	account.Type = f.Type

	if len(f.ParentAccountID) > 0 {
		account.ParentAccountID = &f.ParentAccountID
	}

	if len(f.SegmentID) > 0 {
		account.SegmentID = &f.SegmentID
	}

	if len(f.PortfolioID) > 0 {
		account.PortfolioID = &f.PortfolioID
	}

	if len(f.EntityID) > 0 {
		account.EntityID = &f.EntityID
	}

	account.Status.Code = f.StatusCode

	if len(f.StatusCode) > 0 {
		account.Status.Description = &f.StatusDescription
	}

	var metadata map[string]any

	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	account.Metadata = metadata

	return nil
}

func (f *factoryAccountCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.PortfolioID, "portfolio-id", "", "Specify the portfolio ID.")
	cmd.Flags().StringVar(&f.AssetCode, "asset-code", "", "Specify the asset code associated with this ledger (e.g., USD).")
	cmd.Flags().StringVar(&f.Name, "name", "", "Name the new ledger in your organization.")
	cmd.Flags().StringVar(&f.Alias, "alias", "", "Set an alias for the ledger.")
	cmd.Flags().StringVar(&f.Type, "type", "", "Specify the type of ledger (e.g., PRIMARY or SECONDARY).")
	cmd.Flags().StringVar(&f.ParentAccountID, "parent-account-id", "", "Specify the ID of the parent account.")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "", "Specify the status code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "", "Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.SegmentID, "segment-id", "", "Specify the segment ID.")
	cmd.Flags().StringVar(&f.EntityID, "entity-id", "", "Specify the ID of the associated entity.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing account attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryAccountCreate {
	return &factoryAccountCreate{
		factory:     f,
		repoAccount: rest.NewAccount(f),
		tuiInput:    tui.Input,
	}
}

func newCmdAccountCreate(f *factoryAccountCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates an account.",
		Long: utils.Format(
			"Creates a new account according to the assets available in the user's",
			"Ledger. Replaces the deprecated “Create an Account from Portfolio” ",
			"method. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz account create",
			"$ mdz account create -h",
			"$ mdz account create --json-file payload.json",
			"$ cat payload.json | mdz account create --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
