package organization

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/spf13/cobra"
)

type factoryOrganizationUpdate struct {
	factory         *factory.Factory
	repoOrganiztion repository.Organization
	tuiInput        func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID       string
	LegalName            string
	ParentOrganizationID string
	DoingBusinessAs      string
	LegalDocument        string
	Code                 string
	Description          string
	Line1                string
	Line2                string
	ZipCode              string
	City                 string
	State                string
	Country              string
	Metadata             string
	JSONFile             string
}

func (f *factoryOrganizationUpdate) runE(cmd *cobra.Command, _ []string) error {
	org := mmodel.UpdateOrganizationInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.OrganizationID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &org)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&org)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoOrganiztion.Update(f.OrganizationID, org)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The Organization %s has been successfully updated.", resp.ID))

	return nil
}

func (f *factoryOrganizationUpdate) UpdateRequestFromFlags(org *mmodel.UpdateOrganizationInput) error {
	var err error
	org.LegalName, err = utils.AssignStringField(f.LegalName, "legal-name", f.tuiInput)

	if err != nil {
		return err
	}

	if len(f.ParentOrganizationID) < 1 {
		org.ParentOrganizationID = nil
	} else {
		org.ParentOrganizationID = &f.ParentOrganizationID
	}

	doingBusinessAsPtr, err := utils.AssignStringField(f.DoingBusinessAs, "doing-business-as", f.tuiInput)
	if err != nil {
		return err
	}

	org.DoingBusinessAs = &doingBusinessAsPtr

	org.Status.Code = f.Code
	org.Status.Description = utils.AssignOptionalStringPtr(f.Description)

	org.Address.Line1 = f.Line1
	org.Address.Line2 = utils.AssignOptionalStringPtr(f.Line2)
	org.Address.ZipCode = f.ZipCode
	org.Address.City = f.City
	org.Address.State = f.State

	org.Address.Country, err = utils.AssignStringField(f.Country, "country", f.tuiInput)
	if err != nil {
		return err
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	org.Metadata = metadata

	return nil
}

func (f *factoryOrganizationUpdate) setFlags(cmd *cobra.Command) {
	// Flags for Organization
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID to retrieve details.")
	cmd.Flags().StringVar(&f.LegalName, "legal-name", "", "Legal name of the organization.")
	cmd.Flags().StringVar(&f.ParentOrganizationID, "parent-organization-id", "",
		"ID of the parent organization, if applicable.")
	cmd.Flags().StringVar(&f.DoingBusinessAs, "doing-business-as", "",
		"Optional business name used by the organization.")
	cmd.Flags().StringVar(&f.LegalDocument, "legal-document", "",
		"Legal document number of the organization.")

	// Flags for Status
	cmd.Flags().StringVar(&f.Code, "code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.Description, "description", "",
		"Description of the current status of the organization.")

	// Flags for Address
	cmd.Flags().StringVar(&f.Line1, "line1", "",
		"First line of the address (e.g., street, number).")
	cmd.Flags().StringVar(&f.Line2, "line2", "",
		"Second line of the address (e.g., suite, apartment) - optional.")
	cmd.Flags().StringVar(&f.ZipCode, "zip-code", "", "Postal/ZIP code of the address.")
	cmd.Flags().StringVar(&f.City, "city", "", "City of the organization.")
	cmd.Flags().StringVar(&f.State, "state", "",
		"State or region of the organization.")
	cmd.Flags().StringVar(&f.Country, "country", "",
		"Country of the organization (ISO 3166-1 alpha-2 format).")

	// Flags for Metadata
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")

	// Flags command Update
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being Updated; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryOrganizationUpdate {
	return &factoryOrganizationUpdate{
		factory:         f,
		repoOrganiztion: rest.NewOrganization(f),
		tuiInput:        tui.Input,
	}
}

func newCmdOrganizationUpdate(f *factoryOrganizationUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an existing organization in Midaz",
		Long: "The organization update command allows you to modify an existing organization " +
			"in Midaz. If the parentOrganizationId field is provided, it must match the " +
			"ID of an existing organization; otherwise, it will be ignored. This command " +
			"supports updating organization details like status, address, and metadata fields.",
		Example: utils.Format(
			"$ mdz organization update",
			"$ mdz organization update -h",
			"$ mdz organization update --json-file payload.json",
			"$ cat payload.json | mdz organization Update --organization-id '1234' --json-file -",
			"$ echo '{...}' | mdz organization Update --organization-id '1234' --json-file -",
			"$ mdz organization update --organization-id '1234' --legal-name 'Gislason LLCT' --doing-business-as 'The ledger.io' --legal-document '48784548000104' --code 'ACTIVE' --description 'Test Ledger' --line1 'Av Santso' --line2 'VJ 222' --zip-code '04696040' --city 'West' --state 'VJ' --country 'MG' --metadata '{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
