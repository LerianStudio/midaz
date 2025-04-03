package segment

import (
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"

	"github.com/spf13/cobra"
)

type factorySegmentDelete struct {
	factory        *factory.Factory
	repoSegment    repository.Segment
	tuiInput       func(message string) (string, error)
	OrganizationID string
	LedgerID       string
	SegmentID      string
}

func (f *factorySegmentDelete) ensureFlagInput(cmd *cobra.Command) error {
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

	if !cmd.Flags().Changed("segment-id") && len(f.SegmentID) < 1 {
		id, err := f.tuiInput("Enter your segment-id")

		if err != nil {
			return err
		}

		f.SegmentID = id
	}

	return nil
}

func (f *factorySegmentDelete) runE(cmd *cobra.Command, _ []string) error {
	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	err := f.repoSegment.Delete(f.OrganizationID, f.LedgerID, f.SegmentID)

	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, f.SegmentID, "Segment", output.Deleted)

	return nil
}

func (f *factorySegmentDelete) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.SegmentID, "segment-id", "", "Specify the portfolio ID")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacDelete(f *factory.Factory) *factorySegmentDelete {
	return &factorySegmentDelete{
		factory:     f,
		repoSegment: rest.NewSegment(f),
		tuiInput:    tui.Input,
	}
}

func newCmdSegmentDelete(f *factorySegmentDelete) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Removes an existing segment.",
		Long: utils.Format(
			"The delete subcommand allows you to delete a segment, removing its",
			"settings and segmenting rules. It is useful for deactivating obsolete",
			"segments or adjusting the organization of segments without changing",
			"the structure of customers.",
		),
		Example: utils.Format(
			"$ mdz segment delete --organization-id '1234' --ledger-id '4421' --segment-id '55232'",
			"$ mdz segment delete -i 12314",
			"$ mdz segment delete -h",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
