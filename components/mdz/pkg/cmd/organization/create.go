package organization

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

type factoryOrganizationCreate struct {
	factory         *factory.Factory
	repoOrganiztion repository.Organization
	tuiInput        func(message string) (string, error)
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
	Chave                string
	Bitcoin              string
	Boolean              string
	Double               string
	Int                  string
	JSONFile             string
}

func (f *factoryOrganizationCreate) runE(cmd *cobra.Command, _ []string) error {
	org := model.Organization{}

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

	resp, err := f.repoOrganiztion.Create(org)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The organization_id %s has been successfully created", resp.ID))

	return nil
}

func (f *factoryOrganizationCreate) createRequestFromFlags(org *model.Organization) error {
	org.Address = model.Address{}

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

	org.DoingBusinessAs, err = utils.AssignStringField(f.DoingBusinessAs, "doing-business-as", f.tuiInput)
	if err != nil {
		return err
	}

	org.LegalDocument, err = utils.AssignStringField(f.LegalDocument, "legal-document", f.tuiInput)
	if err != nil {
		return err
	}

	var status *model.Status

	if len(f.Code) > 0 || len(f.Description) > 0 {
		tempStatus := model.Status{}

		tempStatus.Code = utils.AssignOptionalStringPtr(f.Code)
		tempStatus.Description = utils.AssignOptionalStringPtr(f.Description)

		status = &tempStatus
	}

	org.Status = status

	org.Address.Line1 = utils.AssignOptionalStringPtr(f.Line1)
	org.Address.Line2 = utils.AssignOptionalStringPtr(f.Line2)
	org.Address.ZipCode = utils.AssignOptionalStringPtr(f.ZipCode)
	org.Address.City = utils.AssignOptionalStringPtr(f.City)
	org.Address.State = utils.AssignOptionalStringPtr(f.State)

	org.Address.Country, err = utils.AssignStringField(f.Country, "country", f.tuiInput)
	if err != nil {
		return err
	}

	metadata, err := buildMetadataCreate(f)
	if err != nil {
		return err
	}

	org.Metadata = metadata

	return nil
}

func buildMetadataCreate(f *factoryOrganizationCreate) (*model.Metadata, error) {
	if len(f.Chave) == 0 && len(f.Bitcoin) == 0 && len(f.Boolean) == 0 && len(f.Double) == 0 && len(f.Int) == 0 {
		return nil, nil
	}

	tempMetadata := model.Metadata{}

	tempMetadata.Chave = utils.AssignOptionalStringPtr(f.Chave)
	tempMetadata.Bitcoin = utils.AssignOptionalStringPtr(f.Bitcoin)

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

	// Flags for Metadata
	cmd.Flags().StringVar(&f.Chave, "chave", "",
		"Custom metadata key for the organization.")
	cmd.Flags().StringVar(&f.Bitcoin, "bitcoin", "",
		"Bitcoin address or value associated with the organization.")
	cmd.Flags().StringVar(&f.Boolean, "boolean", "",
		"Boolean metadata for custom use.")
	cmd.Flags().StringVar(&f.Double, "double", "",
		"Floating-point number metadata for custom use.")
	cmd.Flags().StringVar(&f.Int, "int", "", "Integer metadata for custom use.")

	// Flags command create
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being created; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryOrganizationCreate {
	return &factoryOrganizationCreate{
		factory:         f,
		repoOrganiztion: rest.NewOrganization(f),
		tuiInput:        tui.Input,
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
			"$ mdz organization create --legal-name 'Gislason LLCT' --doing-business-as 'The ledger.io' --legal-document '48784548000104' --code 'ACTIVE' --description 'Test Ledger' --line1 'Av Santso' --line2 'VJ 222' --zip-code '04696040' --city 'West' --state 'VJ' --country 'MG' --bitcoin '1YLHctiipHZupwrT5sGwuYbks5rn64bm' --boolean true --chave 'metadata_chave' --double '10.5' --int 1",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
