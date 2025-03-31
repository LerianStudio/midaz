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

type factoryAssetRateCreate struct {
	factory       *factory.Factory
	repoAssetRate repository.AssetRate
	tuiInput      func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID string
	LedgerID       string
	FromAssetCode  string
	ToAssetCode    string
	Rate           string
	RateScale      string
	Metadata       string
	JSONFile       string
}

func (f *factoryAssetRateCreate) runE(cmd *cobra.Command, _ []string) error {
	assetRate := mmodel.CreateAssetRateInput{}

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

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &assetRate)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&assetRate)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoAssetRate.Create(f.OrganizationID, f.LedgerID, assetRate)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Asset Rate", output.Created)

	return nil
}

func (f *factoryAssetRateCreate) createRequestFromFlags(assetRate *mmodel.CreateAssetRateInput) error {
	var err error

	assetRate.FromAssetCode, err = utils.AssignStringField(f.FromAssetCode, "from-asset-code", f.tuiInput)
	if err != nil {
		return err
	}

	assetRate.ToAssetCode, err = utils.AssignStringField(f.ToAssetCode, "to-asset-code", f.tuiInput)
	if err != nil {
		return err
	}

	if len(f.Rate) > 0 {
		rate, err := strconv.ParseInt(f.Rate, 10, 64)
		if err != nil {
			return errors.New("Error parsing rate: " + err.Error())
		}

		assetRate.Rate = rate
	} else {
		rateStr, err := f.tuiInput("Enter the rate")
		if err != nil {
			return err
		}

		rate, err := strconv.ParseInt(rateStr, 10, 64)
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
	} else {
		rateScaleStr, err := f.tuiInput("Enter the rate scale (decimal places)")
		if err != nil {
			return err
		}

		rateScale, err := strconv.ParseInt(rateScaleStr, 10, 64)
		if err != nil {
			return errors.New("Error parsing rate scale: " + err.Error())
		}

		assetRate.RateScale = rateScale
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	assetRate.Metadata = metadata

	return nil
}

func (f *factoryAssetRateCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.FromAssetCode, "from-asset-code", "", "Specify the source asset code (e.g., USD).")
	cmd.Flags().StringVar(&f.ToAssetCode, "to-asset-code", "", "Specify the target asset code (e.g., EUR).")
	cmd.Flags().StringVar(&f.Rate, "rate", "", "Specify the exchange rate as an integer (will be scaled by rate-scale).")
	cmd.Flags().StringVar(&f.RateScale, "rate-scale", "", "Specify the scale of the rate (decimal places).")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing asset rate attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryAssetRateCreate {
	return &factoryAssetRateCreate{
		factory:       f,
		repoAssetRate: rest.NewAssetRate(f),
		tuiInput:      tui.Input,
	}
}

func newCmdAssetRateCreate(f *factoryAssetRateCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates an asset rate.",
		Long: utils.Format(
			"Creates a new asset rate in the specified ledger. An asset rate defines",
			"the exchange rate between two assets. Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz asset-rate create",
			"$ mdz asset-rate create -h",
			"$ mdz asset-rate create --from-asset-code USD --to-asset-code EUR --rate 120 --rate-scale 2",
			"$ mdz asset-rate create --json-file payload.json",
			"$ cat payload.json | mdz asset-rate create --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
