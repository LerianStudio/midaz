package operation

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/spf13/cobra"
)

type factoryOperationUpdate struct {
	factory      *factory.Factory
	repoOperation repository.Operation
	tuiInput     func(message string) (string, error)
	flagsUpdate
}

type flagsUpdate struct {
	OrganizationID string
	LedgerID       string
	TransactionID  string
	OperationID    string
	Description    string
	Metadata       string
	JSONFile       string
}

func (f *factoryOperationUpdate) runE(cmd *cobra.Command, _ []string) error {
	operation := mmodel.UpdateOperationInput{}

	if !cmd.Flags().Changed("organization-id") && len(f.OrganizationID) < 1 {
		id, err := f.tuiInput("Enter your organization-id")
		if err != nil {
			return errors.Wrap(err, "failed to get organization ID from input")
		}

		f.OrganizationID = id
	}

	if !cmd.Flags().Changed("ledger-id") && len(f.LedgerID) < 1 {
		id, err := f.tuiInput("Enter your ledger-id")
		if err != nil {
			return errors.Wrap(err, "failed to get ledger ID from input")
		}

		f.LedgerID = id
	}

	if !cmd.Flags().Changed("transaction-id") && len(f.TransactionID) < 1 {
		id, err := f.tuiInput("Enter the transaction-id")
		if err != nil {
			return errors.Wrap(err, "failed to get transaction ID from input")
		}

		f.TransactionID = id
	}

	if !cmd.Flags().Changed("operation-id") && len(f.OperationID) < 1 {
		id, err := f.tuiInput("Enter the operation-id")
		if err != nil {
			return errors.Wrap(err, "failed to get operation ID from input")
		}

		f.OperationID = id
	}

	if cmd.Flags().Changed("json-file") {
		err := utils.FlagFileUnmarshalJSON(f.JSONFile, &operation)
		if err != nil {
			return errors.UserError(err, "Verify if the file format is JSON or fix its content according to the JSON format specification at https://www.json.org/json-en.html")
		}
	} else {
		err := f.createRequestFromFlags(&operation)
		if err != nil {
			return errors.Wrap(err, "failed to create operation update request from flags")
		}
	}

	resp, err := f.repoOperation.Update(f.OrganizationID, f.LedgerID, f.TransactionID, f.OperationID, operation)
	if err != nil {
		return errors.CommandError("operation update", err)
	}

	output.FormatAndPrint(f.factory, resp, "Operation", output.Updated)

	return nil
}

func (f *factoryOperationUpdate) createRequestFromFlags(operation *mmodel.UpdateOperationInput) error {
	var err error

	operation.Description, err = utils.AssignStringField(f.Description, "description", f.tuiInput)
	if err != nil {
		return errors.Wrap(err, "failed to assign description field")
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(f.Metadata), &metadata); err != nil {
		return errors.ValidationError("metadata", "Invalid JSON format for metadata")
	}

	operation.Metadata = metadata

	return nil
}

func (f *factoryOperationUpdate) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.TransactionID, "transaction-id", "", "Specify the transaction ID.")
	cmd.Flags().StringVar(&f.OperationID, "operation-id", "", "Specify the operation ID.")
	cmd.Flags().StringVar(&f.Description, "description", "", "Update the operation description.")
	cmd.Flags().StringVar(&f.Metadata, "metadata", "{}", "Metadata in JSON format, e.g., '{\"key1\": \"value\", \"key2\": 123}'.")
	cmd.Flags().StringVar(&f.JSONFile, "json-file", "", "Path to a JSON file containing operation attributes, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacUpdate(f *factory.Factory) *factoryOperationUpdate {
	return &factoryOperationUpdate{
		factory:      f,
		repoOperation: rest.NewOperation(f),
		tuiInput:     tui.Input,
	}
}

func newCmdOperationUpdate(f *factoryOperationUpdate) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates an operation.",
		Long: utils.Format(
			"Updates a specific operation's attributes, such as its description or metadata.",
			"Returns the updated operation information.",
		),
		Example: utils.Format(
			"$ mdz operation update",
			"$ mdz operation update -h",
			"$ mdz operation update --organization-id 123 --ledger-id 456 --transaction-id 789 --operation-id 012 --description \"Updated description\"",
			"$ mdz operation update --json-file operation-updates.json",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}