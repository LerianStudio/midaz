package organization

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryOrganizationDelete struct {
	factory          *factory.Factory
	repoOrganization repository.Organization
	tuiInput         func(message string) (string, error)
	organizationID   string
}

func (f *factoryOrganizationDelete) runE(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("organization-id") && len(f.organizationID) < 1 {
		id, err := tui.Input("Enter your organization-id")

		if err != nil {
			return err
		}

		f.organizationID = id
	}

	err := f.repoOrganization.Delete(f.organizationID)

	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.organizationID, "Organization", output.Deleted)

	return nil
}

func (f *factoryOrganizationDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.organizationID, "organization-id", "i", "",
		"Specify the organization ID to delete")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryOrganizationDelete {
	return &factoryOrganizationDelete{
		factory:          f,
		repoOrganization: rest.NewOrganization(f),
		tuiInput:         tui.Input,
	}
}

func newCmdOrganizationDelete(f *factoryOrganizationDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove a specific organization in Midaz",
		Long: "The /`organization delete/` command allows you to remove a specific organization in Midaz " +
			"by specifying the organization ID.",
		Example: utils.Format(
			"$ mdz organization delete --organization-id 12312",
			"$ mdz organization delete -i 12314",
			"$ mdz organization delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
