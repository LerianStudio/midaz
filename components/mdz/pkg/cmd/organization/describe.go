package organization

import (
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

type factoryOrganizationDescribe struct {
	factory          *factory.Factory
	repoOrganization repository.Organization
	organizationID   string
	Out              string
	JSON             bool
}

func (f *factoryOrganizationDescribe) runE(cmd *cobra.Command, _ []string) error {
	org, err := f.repoOrganization.GetByID(f.organizationID)
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

func (f *factoryOrganizationDescribe) describePrint(org *mmodel.Organization) {
	tbl := table.New("FIELDS", "VALUES")

	headerFmt := color.New(color.FgYellow).SprintfFunc()
	fieldFmt := color.New(color.FgYellow).SprintfFunc()

	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(fieldFmt)
	tbl.WithWriter(f.factory.IOStreams.Out)

	f.addBasicFields(tbl, org)
	f.addAddressFields(tbl, org)
	f.addStatusFields(tbl, org)
	tbl.AddRow("Metadata:", org.Metadata)

	tbl.Print()
}

func (f *factoryOrganizationDescribe) addBasicFields(tbl table.Table, org *mmodel.Organization) {
	tbl.AddRow("ID:", org.ID)
	tbl.AddRow("Legal Name:", org.LegalName)

	if org.DoingBusinessAs != nil {
		tbl.AddRow("Doing Business As:", *org.DoingBusinessAs)
	}

	tbl.AddRow("Legal Document:", org.LegalDocument)

	if org.ParentOrganizationID != nil {
		tbl.AddRow("Parent Organization ID:", *org.ParentOrganizationID)
	}

	tbl.AddRow("Created At:", org.CreatedAt)
	tbl.AddRow("Update At:", org.UpdatedAt)

	if org.DeletedAt != nil {
		tbl.AddRow("Delete At:", *org.DeletedAt)
	}
}

func (f *factoryOrganizationDescribe) addAddressFields(tbl table.Table, org *mmodel.Organization) {
	tbl.AddRow("Address Line1:", org.Address.Line1)

	if org.Address.Line2 != nil {
		tbl.AddRow("Address Line2:", *org.Address.Line2)
	}

	tbl.AddRow("Address City:", org.Address.City)
	tbl.AddRow("Address State:", org.Address.State)
	tbl.AddRow("Address Country:", org.Address.Country)
}

func (f *factoryOrganizationDescribe) addStatusFields(tbl table.Table, org *mmodel.Organization) {
	tbl.AddRow("Status Code:", org.Status.Code)

	if org.Status.Description != nil {
		tbl.AddRow("Status Description:", *org.Status.Description)
	}
}

func (f *factoryOrganizationDescribe) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Out, "out", "", "Exports the output to the given <file_path/file_name.ext>")
	cmd.Flags().BoolVar(&f.JSON, "json", false, "returns the table in json format")
	cmd.Flags().StringVarP(&f.organizationID, "organization-id", "i", "",
		"Specify the organization ID to retrieve details")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDescribe(f *factory.Factory) *factoryOrganizationDescribe {
	return &factoryOrganizationDescribe{
		factory:          f,
		repoOrganization: rest.NewOrganization(f),
	}
}

func newCmdOrganizationDescribe(f *factoryOrganizationDescribe) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Retrieve details of a specific organization in Midaz",
		Long: "The organization describe command allows you to retrieve detailed information " +
			"about a specific organization in Midaz by specifying the organization ID.",
		Example: utils.Format(
			"$ mdz organization describe --organization-id 12312",
			"$ mdz organization describe -i 12314",
			"$ mdz organization describe -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
