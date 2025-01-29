package cluster

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factoryClusterDelete struct {
	factory        *factory.Factory
	repoCluster    repository.Cluster
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	ClusterID      string
}

func (f *factoryClusterDelete) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("cluster-id") && len(f.ClusterID) < 1 {
		id, err := f.tuiInput("Enter your cluster-id")
		if err != nil {
			return err
		}

		f.ClusterID = id
	}

	return nil
}

func (f *factoryClusterDelete) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	err := f.repoCluster.Delete(f.OrganizationID, f.LedgerID, f.ClusterID)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.ClusterID, "Cluster", output.Deleted)

	return nil
}

func (f *factoryClusterDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.ClusterID, "cluster-id", "", "Specify the portfolio ID")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factoryClusterDelete {
	return &factoryClusterDelete{
		factory:     f,
		repoCluster: rest.NewCluster(f),
		tuiInput:    tui.Input,
	}
}

func newCmdClusterDelete(f *factoryClusterDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Removes an existing cluster.",
		Long: utils.Format(
			"The delete subcommand allows you to delete a cluster, removing its",
			"settings and clustering rules. It is useful for deactivating obsolete",
			"clusters or adjusting the organization of clusters without changing",
			"the structure of customers.",
		),
		Example: utils.Format(
			"$ mdz cluster delete --organization-id '1234' --ledger-id '4421' --cluster-id '55232'",
			"$ mdz cluster delete -i 12314",
			"$ mdz cluster delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
