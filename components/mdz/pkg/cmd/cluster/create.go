package cluster

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

type factoryClusterCreate struct {
	factory     *factory.Factory
	repoProduct repository.Product
	tuiInput    func(message string) (string, error)
	flagsCreate
}

type flagsCreate struct {
	OrganizationID string
	LedgerID       string
	Name           string
	Code           string
	Description    string
	Metadata       string
	JSONFile       string
}

func (f *factoryClusterCreate) runE(cmd *cobra.Command, _ []string) error {
	cluster := mmodel.CreateClusterInput{}

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

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &cluster)
		if err != nil {
			return errors.New("failed to decode the given 'json' file. Verify if " +
				"the file format is JSON or fix its content according to the JSON format " +
				"specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&cluster)
		if err != nil {
			return err
		}
	}

	resp, err := f.repoCluster.Create(f.OrganizationID, f.LedgerID, cluster)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Cluster", output.Created)

	return nil
}

func (f *factoryClusterCreate) createRequestFromFlags(portfolio *mmodel.CreateClusterInput) error {
	var err error

	portfolio.Name, err = utils.AssignStringField(f.Name, "name", f.tuiInput)
	if err != nil {
		return err
	}

	portfolio.Status.Code = f.Code

	if len(f.Description) > 0 {
		portfolio.Status.Description = &f.Description
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.New("Error parsing metadata: " + err.Error())
	}

	portfolio.Metadata = metadata

	return nil
}

func (f *factoryClusterCreate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID,
		"organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID,
		"ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.Name, "name", "",
		"name new ledger your organization")
	cmd.Flags().StringVar(&f.Code, "status-code", "",
		"code for the organization (e.g., ACTIVE).")
	cmd.Flags().StringVar(&f.Description, "status-description", "",
		"Description of the current status of the ledger.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}",
		"Metadata in JSON format, ex: '{\"key1\": \"value\", \"key2\": 123}'")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "",
		`Path to a JSON file containing the attributes of the Cluster being 
		created; you can use - for reading from stdin`)
	cmd.Flags().BoolP("help", "h", false,
		"Displays more information about the Mdz CLI")
}

func newInjectFacCreate(f *factory.Factory) *factoryClusterCreate {
	return &factoryClusterCreate{
		factory:     f,
		repoCluster: rest.NewCluster(f),
		tuiInput:    tui.Input,
	}
}

func newCmdClusterCreate(f *factoryClusterCreate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new cluster for clustering customers.",
		Long: utils.Format(
			"The create subcommand allows you to set up a new cluster, defining",
			"the policies and grouping rules to organize customers according to",
			"specific characteristics. This feature is useful for establishing",
			"new clusters and targeting business strategies at specific groups.",
		),
		Example: utils.Format(
			"$ mdz cluster create",
			"$ mdz cluster create -h",
			"$ mdz cluster create --json-file payload.json",
			"$ cat payload.json | mdz cluster create --json-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
