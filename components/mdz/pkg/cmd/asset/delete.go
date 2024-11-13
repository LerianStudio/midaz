package asset

import (
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/spf13/cobra"
)

type factoryAssetDelete struct {
	factory        *factory.Factory
	repoAsset      repository.Asset
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	AssetID        string
}

func (f *factoryAssetDelete) ensureFlagInput(cmd *cobra.Command) error {
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

func (f *factoryAssetDelete) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	err := f.repoAsset.Delete(f.OrganizationID, f.LedgerID, f.AssetID)
	if err != nil {
		return err
	}

	output.Printf(f.factory.IOStreams.Out,
		fmt.Sprintf("The Asset ID %s has been successfully deleted.", f.AssetID))

	return nil
}

func (f *factoryAssetDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.AssetID, "asset-id", "", "Specify the asset ID")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryAssetDelete {
	return &factoryAssetDelete{
		factory:   f,
		repoAsset: rest.NewAsset(f),
		tuiInput:  tui.Input,
	}
}

func newCmdAssetDelete(f *factoryAssetDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Removes an asset from the ledger.",
		Long: utils.Format(
			"Deletes an asset from the ledger, making it unavailable for future",
			"operations and account balances. Use with caution, as it can affect",
			"accounts and operations that depend on that specific asset.",
		),
		Example: utils.Format(
			"$ mdz asset delete --organization-id '1234' --ledger-id '4421' --asset-id '55232'",
			"$ mdz asset delete -i 12314",
			"$ mdz asset delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
