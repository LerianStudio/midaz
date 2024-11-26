package ledger

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/spf13/cobra"
)

type factoryLedgerUpdate struct {
	factory    *factory.Factory
	repoLedger repository.Ledger
	tuiInput   func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID string
	LedgerID       string
	Name           string
	Code           string
	Description    string
	Metadata       string
	JSONFile       string
}

func (f *factoryLedgerUpdate) runE(cmd *cobra.Command, _ []string) error {
	led := mmodel.UpdateLedgerInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.LedgerID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &led)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&led)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoLedger.Update(f.OrganizationID, f.LedgerID, led)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The Ledger %s has been successfully updated.", resp.ID))

	return nil
}

func (f *factoryLedgerUpdate) UpdateRequestFromFlags(led *mmodel.UpdateLedgerInput) error {
	led.Name = f.Name
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

func (f *factoryLedgerUpdate) setFlags(cmd *cobra.Command) {
	// Flags for Ledger
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID to retrieve details.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID to retrieve details.")

	cmd.Flags().StringVar(&f.Name, "name", "", "Legal name of the Ledger.")

	// Flags for Status
	cmd.Flags().StringVar(&f.Code, "code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.Description, "description", "",
		"Description of the current status of the organization.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")

	// Flags command Update
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being Updated; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryLedgerUpdate {
	return &factoryLedgerUpdate{
		factory:    f,
		repoLedger: rest.NewLedger(f),
		tuiInput:   tui.Input,
	}
}

func newCmdLedgerUpdate(f *factoryLedgerUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates information from an existing ledger",
		Long: `It allows the details of a ledger to be updated, such as configuration 
			changes or adjustments needed for better transaction management.`,
		Example: utils.Format(
			"$ mdz ledger update",
			"$ mdz ledger update -h",
			"$ mdz ledger update --json-file payload.json",
			"$ cat payload.json | mdz ledger Update --organization-id '1234' --ledger-id '4421' --json-file -",
			"$ mdz ledger update --organization-id '1234' --ledger-id '4421' --legal-name 'Gislason LLCT' --doing-business-as 'The ledger.io' --legal-document '48784548000104' --code 'ACTIVE' --description 'Test Ledger' --line1 'Av Santso' --line2 'VJ 222' --zip-code '04696040' --city 'West' --state 'VJ' --country 'MG' --metadata '{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
