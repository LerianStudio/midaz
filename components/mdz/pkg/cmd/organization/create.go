package organization

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryOrganizationCreate struct {
	factory          *factory.Factory
	repoOrganization repository.Organization
	tuiInput         func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
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

func (f *factoryOrganizationCreate) runE(cmd *cobra.Command, _ []string) error {
	org := mmodel.CreateOrganizationInput{}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &org)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&org)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoOrganization.Create(org)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The organization_id %s has been successfully created", resp.ID))

	return nil
}

func (f *factoryOrganizationCreate) createRequestFromFlags(org *mmodel.CreateOrganizationInput) error {
	org.Address = mmodel.Address{}

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

	org.LegalDocument, err = utils.AssignStringField(f.LegalDocument, "legal-document", f.tuiInput)
	if err != nil {
		return err
	}

	org.Status.Code = f.Code
	org.Status.Description = utils.AssignOptionalStringPtr(f.Description)

	org.Address.Line1 = f.Line1

	if len(f.Line2) > 0 {
		org.Address.Line2 = &f.Line2
	}

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

func (f *factoryOrganizationCreate) setFlags(cmd *cobra.Command) {
	// Flags for Organization
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
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")

	// Flags command create
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being created; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryOrganizationCreate {
	return &factoryOrganizationCreate{
		factory:          f,
		repoOrganization: rest.NewOrganization(f),
		tuiInput:         tui.Input,
	}
}

func newCmdOrganizationCreate(f *factoryOrganizationCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new organization in Midaz",
		Long: "The organization create command allows you to create a new organization " +
			"in Midaz. If the parentOrganizationId field is sent, it must match the " +
			"ID of an existing organization, otherwise it will be ignored.",
		Example: utils.Format(
			"$ mdz organization create",
			"$ mdz organization create -h",
			"$ mdz organization create --json-file payload.json",
			"$ cat payload.json | mdz organization create --json-file -",
			"$ echo '{...}' | mdz organization create --json-file -",
			"$ mdz organization create --legal-name 'Gislason LLCT' --doing-business-as 'The ledger.io' --legal-document '48784548000104' --code 'ACTIVE' --description 'Test Ledger' --line1 'Av Santso' --line2 'VJ 222' --zip-code '04696040' --city 'West' --state 'VJ' --country 'MG' --metadata '{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
