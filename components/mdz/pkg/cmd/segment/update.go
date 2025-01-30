package segment

import (
	"encoding/json"
	"errors"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factorySegmentUpdate struct {
	factory     *factory.Factory
	repoSegment repository.Segment
	tuiInput    func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID    string
	LedgerID          string
	SegmentID         string
	Name              string
	StatusCode        string
	StatusDescription string
	Metadata          string
	JSONFile          string
}

func (f *factorySegmentUpdate) ensureFlagInput(cmd *cobra.Command) error {
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

func (f *factorySegmentUpdate) runE(cmd *cobra.Command, _ []string) error {
	Segment := mmodel.UpdateSegmentInput{}

	if err := f.ensureFlagInput(cmd); err != nil {
		return err
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &Segment)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.UpdateRequestFromFlags(&Segment)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoSegment.Update(f.OrganizationID, f.LedgerID, f.SegmentID, Segment)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Segment", output.Updated)

	return nil
}

func (f *factorySegmentUpdate) UpdateRequestFromFlags(portfolio *mmodel.UpdateSegmentInput) error {
	portfolio.Name = f.Name
	portfolio.Status.Code = f.StatusCode

	if len(f.StatusDescription) > 0 {
		portfolio.Status.Description = &f.StatusDescription
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	portfolio.Metadata = metadata

	return nil
}

func (f *factorySegmentUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID")
	cmd.Flags().StringVar(&f.SegmentID, "segment-id", "", "Specify the portfolio ID")
	cmd.Flags().StringVar(&f.Name, "name", "", "Legal name of the Segment.")
	cmd.Flags().StringVar(&f.StatusCode, "status-code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.StatusDescription, "status-description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")

	// Flags command Update
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing "+
		"the attributes of the Organization being Updated; you can use - for reading from stdin")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factorySegmentUpdate {
	return &factorySegmentUpdate{
		factory:     f,
		repoSegment: rest.NewSegment(f),
		tuiInput:    tui.Input,
	}
}

func newCmdSegmentUpdate(f *factorySegmentUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates an existing segment with new policies.",
		Long: utils.Format(
			"The update subcommand allows you to adjust the policies and settings",
			"of an existing segment. With it, you can modify the segmenting rules,",
			"adapting the grouping of clients according to changes in business",
			"strategies and needs.",
		),
		Example: utils.Format(
			"$ mdz segment update",
			"$ mdz segment update -h",
			"$ mdz segment update --json-file payload.json",
			"$ cat payload.json | mdz segment update --organization-id '1234' --ledger-id '4421' --segment-id '45232' --json-file -",
			"$ mdz portfolio update --organization-id '1234' --ledger-id '4421' --portfolio-id '55232' --name 'Gislason LLCT'",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
