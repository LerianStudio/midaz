package ledger

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

type factoryLedgerCreate struct {
	factory    *factory.Factory
	repoLedger repository.Ledger
	tuiInput   func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID string
	Name           string
	Code           string
	Description    string
	Metadata       string
	JSONFile       string
}

func (f *factoryLedgerCreate) runE(cmd *cobra.Command, _ []string) error {
	led := mmodel.CreateLedgerInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := f.tuiInput("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &led)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&led)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoLedger.Create(f.OrganizationID, led)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Ledger", output.Created)

	return nil
}

func (f *factoryLedgerCreate) createRequestFromFlags(led *mmodel.CreateLedgerInput) error {
	var err error

	led.Name, err = utils.AssignStringField(f.Name, "name", f.tuiInput)
	if err != nil {
		return err
	}

	led.Status.Code = f.Code

	if len(f.Description) > 0 {
		led.Status.Description = &f.Description
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	led.Metadata = metadata

	return nil
}

func (f *factoryLedgerCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID,
		"organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.Name, "name", "",
		"name new ledger your organization")
	cmd.Flags().StringVar(&f.Code, "code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.Description, "description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "",
		`Path to a JSON file containing the attributes of the Ledger being 
		created; you can use - for reading from stdin`)
	cmd.Flags().BoolP("help", "h", false,
		"Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryLedgerCreate {
	return &factoryLedgerCreate{
		factory:    f,
		repoLedger: rest.NewLedger(f),
		tuiInput:   tui.Input,
	}
}

func newCmdLedgerCreate(f *factoryLedgerCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new ledger for the organization",
		Long: `It creates a ledger within the organization, allowing the creation 
			of multiple records to separate and organize transactions by context, 
			such as geographical location or business units.`,
		Example: utils.Format(
			"$ mdz ledger create",
			"$ mdz ledger create -h",
			"$ mdz ledger create --json-file payload.json",
			"$ cat payload.json | mdz ledger create --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
