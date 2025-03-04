package assetrate

import (
	"encoding/json"
	"strconv"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"

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
	From           string
	To             string
	Rate           string
	Scale          string
	Source         string
	TTL            string
	ExternalID     string
	Metadata       string
	JSONFile       string
}

func (f *factoryAssetRateCreate) runE(cmd *cobra.Command, _ []string) error {
	assetRate := mmodel.CreateAssetRateInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := f.tuiInput("Enter your organization-id")
		if err != nil {
			return errors.Wrap(err, "failed to get organization ID from input")
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := f.tuiInput("Enter your ledger-id")
		if err != nil {
			return errors.Wrap(err, "failed to get ledger ID from input")
		}

		f.LedgerID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &assetRate)
		if err != nil {
			return errors.UserError(err, "Verify if the file format is JSON or fix its content according to the JSON format specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&assetRate)
		if err != nil {
			return errors.Wrap(err, "failed to create asset rate request from flags")
		}
	}

	resp, err := f.repoAssetRate.Create(f.OrganizationID, f.LedgerID, assetRate)
	if err != nil {
		return errors.CommandError("assetrate create", err)
	}

	output.FormatAndPrint(f.factory, resp, "AssetRate", output.Created)

	return nil
}

func (f *factoryAssetRateCreate) createRequestFromFlags(assetRate *mmodel.CreateAssetRateInput) error {
	var err error

	assetRate.From, err = utils.AssignStringField(f.From, "from", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign from field")
	}

	assetRate.To, err = utils.AssignStringField(f.To, "to", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign to field")
	}

	if len(f.Rate) > 0 {
		rate, err := strconv.Atoi(f.Rate)
		if err != nil {
			return errors.ValidationError("rate", "Invalid rate format, must be an integer")
		}

		assetRate.Rate = rate
	} else {
		rateStr, err := f.tuiInput("Enter the rate (integer)")
		if err != nil {
			return errors.Wrap(err, "failed to get rate from input")
		}

		rate, err := strconv.Atoi(rateStr)
		if err != nil {
			return errors.ValidationError("rate", "Invalid rate format, must be an integer")
		}

		assetRate.Rate = rate
	}

	if len(f.Scale) > 0 {
		scale, err := strconv.Atoi(f.Scale)
		if err != nil {
			return errors.ValidationError("scale", "Invalid scale format, must be an integer")
		}

		assetRate.Scale = scale
	}

	if len(f.Source) > 0 {
		assetRate.Source = mpointers.String(f.Source)
	}

	if len(f.TTL) > 0 {
		ttl, err := strconv.Atoi(f.TTL)
		if err != nil {
			return errors.ValidationError("ttl", "Invalid TTL format, must be an integer")
		}

		assetRate.TTL = mpointers.Int(ttl)
	}

	if len(f.ExternalID) > 0 {
		assetRate.ExternalID = mpointers.String(f.ExternalID)
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.ValidationError("metadata", "Invalid JSON format for metadata")
	}

	assetRate.Metadata = metadata

	return nil
}

func (f *factoryAssetRateCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.From, "from", "", "Source asset code (e.g., USD).")
	cmd.Flags().StringVar(&f.To, "to", "", "Target asset code (e.g., BRL).")
	cmd.Flags().StringVar(&f.Rate, "rate", "", "Conversion rate as an integer.")
	cmd.Flags().StringVar(&f.Scale, "scale", "", "Decimal scale for the rate (e.g., 2 means divide by 100).")
	cmd.Flags().StringVar(&f.Source, "source", "", "Source of the exchange rate.")
	cmd.Flags().StringVar(&f.TTL, "ttl", "", "Time to live in seconds.")
	cmd.Flags().StringVar(&f.ExternalID, "external-id", "", "External identifier for the rate.")
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
			"Creates a new asset rate for currency conversion between assets. Allows for",
			"specifying conversion rate, scale, and TTL. Returns the created asset rate.",
		),
		Example: utils.Format(
			"$ mdz assetrate create",
			"$ mdz assetrate create -h",
			"$ mdz assetrate create --from USD --to BRL --rate 500 --scale 2",
			"$ mdz assetrate create --json-file rate-data.json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
