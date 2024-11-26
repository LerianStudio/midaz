package asset

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

type factoryAssetCreate struct {
	factory   *factory.Factory
	repoAsset repository.Asset
	tuiInput  func(message string) (string, error)
	tuiSelect func(message string, options []string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID    string
	LedgerID          string
	Name              string
	Type              string
	Code              string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factoryAssetCreate) runE(cmd *cobra.Command, _ []string) error {
	ass := mmodel.CreateAssetInput{}

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

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &ass)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&ass)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoAsset.Create(f.OrganizationID, f.LedgerID, ass)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The Asset ID %s has been successfully created", resp.ID))

	return nil
}

func (f *factoryAssetCreate) createRequestFromFlags(ass *mmodel.CreateAssetInput) error {
	var err error

	ass.Name, err = utils.AssignStringField(f.Name, "name", f.tuiInput)
	if err != nil {
		return err
	}

	ass.Type, err = utils.AssignStringField(f.Type, "type", f.tuiInput)
	if err != nil {
		return err
	}

	ass.Code, err = utils.AssignStringField(f.Code, "code", f.tuiInput)
	if err != nil {
		return err
	}

	ass.Status.Code = f.StatusCode
	ass.Status.Description = &f.StatusDescription

	if len(f.StatusDescription) > 0 {
		ass.Status.Description = &f.StatusDescription
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	ass.Metadata = metadata

	return nil
}

func (f *factoryAssetCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID,
		"organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID,
		"ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.Name, "name", "",
		"name new ledger your organization")
	cmd.Flags().StringVar(&f.Type, "type", "", "Defines the asset category, Example of the use: https://github.com/LerianStudio/midaz/blob/main/common/utils.go#L91")
	cmd.Flags().StringVar(&f.Code, "code", "", "Asset identifier code Example of the use: https://github.com/LerianStudio/midaz/blob/main/common/utils.go#L114")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "",
		`Path to a JSON file containing the attributes of the Asset being 
		created; you can use - for reading from stdin`)
	cmd.Flags().BoolP("help", "h", false,
		"Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryAssetCreate {
	return &factoryAssetCreate{
		factory:   f,
		repoAsset: rest.NewAsset(f),
		tuiInput:  tui.Input,
		tuiSelect: tui.Select,
	}
}

func newCmdAssetCreate(f *factoryAssetCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new permitted asset in the ledger.",
		Long: utils.Format(
			"Adds a new asset to the ledger to be used as a balance in accounts",
			"and operations. This asset can represent currencies, commodities or",
			"any permitted asset, such as BRL, EUR, BTC, soybeans, among others",
			"making it easier to use in the portfolio.",
		),
		Example: utils.Format(
			"$ mdz asset create",
			"$ mdz asset create -h",
			"$ mdz asset create --json-file payload.json",
			"$ cat payload.json | mdz asset create --json-file -",
			"$ mdz asset create --organization-id 123 --ledger-id 432 --name novonome --code BRL --type currency",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
