package asset

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

type factoryAssetUpdate struct {
	factory   *factory.Factory
	repoAsset repository.Asset
	tuiInput  func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	AssetID           string
	Name              string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factoryAssetUpdate) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("asset-id") && len(f.AssetID) < 1 {
		id, err := tui.Input("Enter your asset-id")
		if err != nil {
			return err
		}

		f.AssetID = id
	}

	return nil
}

func (f *factoryAssetUpdate) runE(cmd *cobra.Command, _ []string) error {
	asset := mmodel.UpdateAssetInput{}

	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &asset)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&asset)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoAsset.Update(f.OrganizationID, f.LedgerID, f.AssetID, asset)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The Asset ID %s has been successfully updated.", resp.ID))

	return nil
}

func (f *factoryAssetUpdate) UpdateRequestFromFlags(asset *mmodel.UpdateAssetInput) error {
	asset.Name = f.Name
	asset.Status.Code = f.StatusCode

	if len(f.StatusDescription) > 0 {
		asset.Status.Description = &f.StatusDescription
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	asset.Metadata = metadata

	return nil
}

func (f *factoryAssetUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.AssetID, "asset-id", "", "Specify the asset ID to retrieve details")
	cmd.Flags().StringVar(&f.Name, "name", "", "Legal name of the Asset.")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")

	// Flags command Update
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being Updated; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryAssetUpdate {
	return &factoryAssetUpdate{
		factory:   f,
		repoAsset: rest.NewAsset(f),
		tuiInput:  tui.Input,
	}
}

func newCmdAssetUpdate(f *factoryAssetUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates the information of an existing asset.",
		Long: utils.Format(
			"you to modify the details of a ledger asset, such as its identifier",
			"or other associated attributes. Ideal for correcting or adjusting ",
			"information on assets already in use.",
		),
		Example: utils.Format(
			"$ mdz asset update",
			"$ mdz asset update -h",
			"$ mdz asset update --json-file payload.json",
			"$ cat payload.json | mdz asset update --organization-id '1234' --ledger-id '4421' --asset-id '45232' --json-file -",
			"$ mdz asset update --organization-id '1234' --ledger-id '4421' --asset-id '55232' --name 'Gislason LLCT'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
