package organization

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/spf13/cobra"
)

type factoryOrganization struct {
	factory *factory.Factory
}

func (f *factoryOrganization) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdOrganizationCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdOrganizationList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdOrganizationDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdOrganizationUpdate(newInjectFacUpdate(f.factory)))
}

func NewCmdOrganization(f *factory.Factory) *cobra.Command {
	fOrg := factoryOrganization{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "organization",
		Short: "Manages organizations at Midaz",
		Long: "The organization command allows you to create and manage organizations " +
			"in the Midaz system. Organizations represent companies that use Midaz, " +
			"such as banks or other institutions.",
		Example: utils.Format(
			"$ mdz organization",
			"$ mdz organization -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
