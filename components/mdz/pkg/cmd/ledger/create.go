package ledger

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
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
	Chave          string
	Bitcoin        string
	Boolean        string
	Double         string
	Int            string
	JSONFile       string
}

func (f *factoryLedgerCreate) runE(cmd *cobra.Command, _ []string) error {
	led := model.LedgerInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
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

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The ledger_id %s has been successfully created", resp.ID))

	return nil
}

func (f *factoryLedgerCreate) createRequestFromFlags(led *model.LedgerInput) error {
	var err error

	led.Name, err = utils.AssignStringField(f.Name, "name", f.tuiInput)
	if err != nil {
		return err
	}

	var status *model.LedgerStatus

	if len(f.Code) > 0 || len(f.Description) > 0 {
		tempStatus := model.LedgerStatus{}

		if len(f.Code) > 0 {
			tempStatus.Code = &f.Code
		}

		if len(f.Description) > 0 {
			tempStatus.Description = &f.Description
		}

		status = &tempStatus
	}

	led.Status = status

	metadata, err := buildMetadata(f)
	if err != nil {
		return err
	}

	led.Metadata = metadata

	return nil
}

func buildMetadata(f *factoryLedgerCreate) (*model.LedgerMetadata, error) {
	if len(f.Chave) == 0 && len(f.Bitcoin) == 0 && len(f.Boolean) == 0 && len(f.Double) == 0 && len(f.Int) == 0 {
		return nil, nil
	}

	tempMetadata := model.LedgerMetadata{}

	if len(f.Chave) > 0 {
		tempMetadata.Chave = &f.Chave
	}

	if len(f.Bitcoin) > 0 {
		tempMetadata.Bitcoin = &f.Bitcoin
	}

	if len(f.Boolean) > 0 {
		var err error

		tempMetadata.Boolean, err = utils.ParseAndAssign(f.Boolean, strconv.ParseBool)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean field: %v", err)
		}
	}

	if len(f.Double) > 0 {
		var err error
		tempMetadata.Double, err = utils.ParseAndAssign(f.Double, func(s string) (float64, error) {
			return strconv.ParseFloat(s, 64)
		})

		if err != nil {
			return nil, fmt.Errorf("invalid double field: %v", err)
		}
	}

	if len(f.Int) > 0 {
		var err error

		tempMetadata.Int, err = utils.ParseAndAssign(f.Int, strconv.Atoi)
		if err != nil {
			return nil, fmt.Errorf("invalid int field: %v", err)
		}
	}

	return &tempMetadata, nil
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
	cmd.Flags().StringVar(&f.Chave, "chave", "",
		"Custom metadata key for the ledger.")
	cmd.Flags().StringVar(&f.Bitcoin, "bitcoin", "",
		"Bitcoin address or value associated with the ledger.")
	cmd.Flags().StringVar(&f.Boolean, "boolean", "",
		"Boolean metadata for custom use.")
	cmd.Flags().StringVar(&f.Double, "double", "",
		"Floating-point number metadata for custom use.")
	cmd.Flags().StringVar(&f.Int, "int", "",
		"Integer metadata for custom use.")
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
