package ledger

import (
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/common/mmodel"
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

type factoryLedgerDescribe struct {
	factory        *factory.Factory
	repoLedger     repository.Ledger
	organizationID string
	ledgerID       string
	Out            string
	JSON           bool
}

func (f *factoryLedgerDescribe) runE(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("organization-id") && len(f.organizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")
		if err != nil {
			return err
		}

		f.organizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.ledgerID) < 1 {
		id, err := tui.Input("Enter your ledger-id")
		if err != nil {
			return err
		}

		f.organizationID = id
	}

	org, err := f.repoLedger.GetByID(f.organizationID, f.ledgerID)
	if err != nil {
		return err
	}

	if f.JSON || cmd.Flags().Changed("out") {
		b, err := json.Marshal(org)
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

	f.describePrint(org)

	return nil
}

func (f *factoryLedgerDescribe) describePrint(led *mmodel.Ledger) {
	tbl := table.New("FIELDS", "VALUES")

	headerFmt := color.New(color.FgYellow).SprintfFunc()
	fieldFmt := color.New(color.FgYellow).SprintfFunc()

	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	tbl.WithWriter(f.factory.IOStreams.Out)

	tbl.AddRow("ID:", led.ID)
	tbl.AddRow("Name :", led.Name)
	tbl.AddRow("Organization ID:", led.OrganizationID)

	tbl.AddRow("Status Code:", led.Status.Code)

	if led.Status.Description != nil {
		tbl.AddRow("Status Description:", *led.Status.Description)
	}

	tbl.AddRow("Created At:", led.CreatedAt)
	tbl.AddRow("Update At:", led.UpdatedAt)

	if led.DeletedAt != nil {
		tbl.AddRow("Delete At:", *led.DeletedAt)
	}

	tbl.AddRow("Metadata:", led.Metadata)

	tbl.Print()
}

func (f *factoryLedgerDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Out, "out", "", "Exports the output to the given <file_path/file_name.ext>")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().StringVar(&f.organizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.ledgerID, "ledger-id", "",
		"Specify the ledger ID to retrieve details")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryLedgerDescribe {
	return &factoryLedgerDescribe{
		factory:    f,
		repoLedger: rest.NewLedger(f),
	}
}

func newCmdLedgerDescribe(f *factoryLedgerDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Displays details of a specific ledger",
		Long: `Provides a detailed view of a selected ledger, including its 
           transactions and operations. Ideal for analyzing information in a 
           single ledger.`,
		Example: utils.Format(
			"$ mdz ledger describe --organization-id 12341234 --ledger-id 12312",
			"$ mdz ledger describe",
			"$ mdz ledger describe -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
