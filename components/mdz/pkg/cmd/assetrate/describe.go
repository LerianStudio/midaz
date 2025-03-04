package assetrate

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryAssetRateDescribe struct {
	factory       *factory.Factory
	repoAssetRate repository.AssetRate
	tuiInput      func(message string) (string, error)
	flagsDescribe
}

type flagsDescribe struct {
	OrganizationID string
	LedgerID       string
	ExternalID     string
}

func (f *factoryAssetRateDescribe) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("external-id") && len(f.ExternalID) < 1 {
		id, err := f.tuiInput("Enter the external-id of the asset rate")
		if err != nil {
			return errors.Wrap(err, "failed to get external ID from input")
		}

		f.ExternalID = id
	}

	assetRate, err := f.repoAssetRate.GetByExternalID(f.OrganizationID, f.LedgerID, f.ExternalID)
	if err != nil {
		return errors.CommandError("assetrate describe", err)
	}

	output.FormatAndPrint(f.factory, assetRate, "", "")

	return nil
}

func (f *factoryAssetRateDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.ExternalID, "external-id", "", "External ID of the asset rate to describe.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryAssetRateDescribe {
	return &factoryAssetRateDescribe{
		factory:       f,
		repoAssetRate: rest.NewAssetRate(f),
		tuiInput:      tui.Input,
	}
}

func newCmdAssetRateDescribe(f *factoryAssetRateDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Describes an asset rate.",
		Long: utils.Format(
			"Retrieves detailed information about a specific asset rate using its external ID. Returns the",
			"rate details including conversion rate, scale, and metadata.",
		),
		Example: utils.Format(
			"$ mdz assetrate describe",
			"$ mdz assetrate describe -h",
			"$ mdz assetrate describe --organization-id 123 --ledger-id 456 --external-id abcd1234",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
