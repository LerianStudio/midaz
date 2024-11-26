package asset

import (
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryAssetDescribe struct {
	factory        *factory.Factory
	repoAsset      repository.Asset
	OrganizationID string
	LedgerID       string
	AssetID        string
	Out            string
	JSON           bool
}

func (f *factoryAssetDescribe) ensureFlagInput(cmd *cobra.Command) error {
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

func (f *factoryAssetDescribe) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	asset, err := f.repoAsset.GetByID(f.OrganizationID, f.LedgerID, f.AssetID)
	if err != nil {
		return err
	}

	return f.outputAsset(cmd, asset)
}

func (f *factoryAssetDescribe) outputAsset(cmd *cobra.Command, asset *mmodel.Asset) error {
	if f.JSON || cmd.Flags().Changed("out") {
		b, err := json.Marshal(asset)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("out") {
			if len(f.Out) == 0 {
				return errors.New("the file path was not entered")
			}

			err = utils.WriteDetailsToFile(b, f.Out)
			if err != nil {
				return errors.New("failed when trying to write the output file " + err.Error())
			}

			output.Printf(f.factory.IOStreams.Out, "File successfully written to: "+f.Out)

			return nil
		}

		output.Printf(f.factory.IOStreams.Out, string(b))

		return nil
	}

	f.describePrint(asset)

	return nil
}

func (f *factoryAssetDescribe) describePrint(asset *mmodel.Asset) {
	tbl := table.New("FIELDS", "VALUES")

	headerFmt := color.New(color.FgYellow).SprintfFunc()
	fieldFmt := color.New(color.FgYellow).SprintfFunc()

	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	tbl.WithWriter(f.factory.IOStreams.Out)

	tbl.AddRow("ID:", asset.ID)
	tbl.AddRow("Name:", asset.Name)
	tbl.AddRow("Type:", asset.Type)
	tbl.AddRow("Code:", asset.Code)
	tbl.AddRow("Status Code:", asset.Status.Code)

	if asset.Status.Description != nil {
		tbl.AddRow("Status Description:", *asset.Status.Description)
	}

	tbl.AddRow("Organization ID:", asset.OrganizationID)
	tbl.AddRow("Ledger ID:", asset.LedgerID)
	tbl.AddRow("Created At:", asset.CreatedAt)
	tbl.AddRow("Update At:", asset.UpdatedAt)

	if asset.DeletedAt != nil {
		tbl.AddRow("Delete At:", *asset.DeletedAt)
	}

	tbl.AddRow("Metadata:", asset.Metadata)

	tbl.Print()
}

func (f *factoryAssetDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Out, "out", "", "Exports the output to the given <file_path/file_name.ext>")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.AssetID, "asset-id", "", "Specify the asset ID to retrieve details")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryAssetDescribe {
	return &factoryAssetDescribe{
		factory:   f,
		repoAsset: rest.NewAsset(f),
	}
}

func newCmdAssetDescribe(f *factoryAssetDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Shows details of a specific asset.",
		Long: utils.Format(
			"Displays detailed information about a selected asset, such as ",
			"identifier, status and other attributes. Useful for checking ",
			"specific characteristics of an asset before using it in ",
			"operations and accounts.",
		),
		Example: utils.Format(
			"$ mdz asset describe --organization-id 12341234 --ledger-id 12312 --asset-id 432123",
			"$ mdz asset describe",
			"$ mdz asset describe -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
