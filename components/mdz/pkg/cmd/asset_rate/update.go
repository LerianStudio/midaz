package assetrate

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
	"strconv"

	"github.com/spf13/cobra"
)

type factoryAssetRateUpdate struct {
	factory       *factory.Factory
	repoAssetRate repository.AssetRate
	tuiInput      func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	AssetRateID       string
	Rate              string
	RateScale         string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factoryAssetRateUpdate) runE(cmd *cobra.Command, _ []string) error {
	assetRate := mmodel.UpdateAssetRateInput{}

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

	if !cmd.Flags().Changed("asset-rate-id") && len(f.AssetRateID) < 1 {
		id, err := f.tuiInput("Enter your asset-rate-id")
		if err != nil {
			return err
		}

		f.AssetRateID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &assetRate)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.updateRequestFromFlags(&assetRate)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoAssetRate.Update(f.OrganizationID, f.LedgerID, f.AssetRateID, assetRate)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Asset Rate", output.Updated)

	return nil
}

func (f *factoryAssetRateUpdate) updateRequestFromFlags(assetRate *mmodel.UpdateAssetRateInput) error {
	if len(f.Rate) > 0 {
		rate, err := strconv.ParseInt(f.Rate, 10, 64)
		if err != nil {
			return errors.New("Error parsing rate: " + err.Error())
		}

		assetRate.Rate = rate
	}

	if len(f.RateScale) > 0 {
		rateScale, err := strconv.ParseInt(f.RateScale, 10, 64)
		if err != nil {
			return errors.New("Error parsing rate scale: " + err.Error())
		}

		assetRate.RateScale = rateScale
	}

	if len(f.StatusCode) > 0 {
		assetRate.Status = mmodel.Status{
			Code: f.StatusCode,
		}

		if len(f.StatusDescription) > 0 {
			description := f.StatusDescription
			assetRate.Status.Description = &description
		}
	}

	if len(f.Metadata) > 0 && f.Metadata != "{}" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
			return errors.New("Error parsing metadata: " + err.Error())
		}

		assetRate.Metadata = metadata
	}

	return nil
}

func (f *factoryAssetRateUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.AssetRateID, "asset-rate-id", "", "Specify the asset rate ID.")
	cmd.Flags().StringVar(&f.Rate, "rate", "", "Specify the exchange rate as an integer (will be scaled by rate-scale).")
	cmd.Flags().StringVar(&f.RateScale, "rate-scale", "", "Specify the scale of the rate (decimal places).")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "", "Specify the status code for the asset rate (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "", "Description of the asset rate status.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing asset rate attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryAssetRateUpdate {
	return &factoryAssetRateUpdate{
		factory:       f,
		repoAssetRate: rest.NewAssetRate(f),
		tuiInput:      tui.Input,
	}
}

func newCmdAssetRateUpdate(f *factoryAssetRateUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates an asset rate.",
		Long: utils.Format(
			"Updates an existing asset rate in the specified ledger.",
			"Only the specified fields will be updated. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz asset-rate update",
			"$ mdz asset-rate update -h",
			"$ mdz asset-rate update --asset-rate-id <id> --rate 125 --rate-scale 2",
			"$ mdz asset-rate update --json-file payload.json",
			"$ cat payload.json | mdz asset-rate update --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
