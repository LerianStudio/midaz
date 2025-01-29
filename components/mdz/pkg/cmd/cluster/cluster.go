package cluster

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"

	"github.com/spf13/cobra"
)

type factoryCluster struct {
	factory *factory.Factory
}

func (f *factoryCluster) setCmds(cmd *cobra.Command) {
	cmd.AddCommand(newCmdClusterCreate(newInjectFacCreate(f.factory)))
	cmd.AddCommand(newCmdClusterList(newInjectFacList(f.factory)))
	cmd.AddCommand(newCmdClusterDescribe(newInjectFacDescribe(f.factory)))
	cmd.AddCommand(newCmdClusterUpdate(newInjectFacUpdate(f.factory)))
	cmd.AddCommand(newCmdClusterDelete(newInjectFacDelete(f.factory)))
}

func NewCmdCluster(f *factory.Factory) *cobra.Command {
	fOrg := factoryCluster{
		factory: f,
	}
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Shows details of a specific cluster.",
		Long: utils.Format(
			"The describe subcommand provides a detailed view of a specific",
			"cluster, including its clustering policies and the rules applied.",
			"Ideal for detailed analysis and monitoring of the settings governing",
			"the grouping of customers within each cluster.",
		),
		Example: utils.Format(
			"$ mdz cluster",
			"$ mdz cluster -h",
		),
	}
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Midaz CLI")
	fOrg.setCmds(cmd)

	return cmd
}
