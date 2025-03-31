package transaction

import (
	"errors"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/output"
	"github.com/LerianStudio/midaz/components/mdz/pkg/tui"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type factoryTransactionCreateDSL struct {
	factory         *factory.Factory
	repoTransaction repository.Transaction
	tuiInput        func(message string) (string, error)
	flagsCreateDSL
}

type flagsCreateDSL struct {
	OrganizationID string
	LedgerID       string
	DSLFile        string
}

func (f *factoryTransactionCreateDSL) runE(cmd *cobra.Command, _ []string) error {
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

	if !cmd.Flags().Changed("dsl-file") && len(f.DSLFile) < 1 {
		file, err := f.tuiInput("Enter the path to your DSL file")
		if err != nil {
			return err
		}

		f.DSLFile = file
	}

	// Read DSL content from file
	var dslContent string

	if f.DSLFile == "-" {
		// Read from stdin
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return errors.New("reading from stdin: " + err.Error())
		}

		dslContent = string(bytes)
	} else {
		// Read from file
		bytes, err := os.ReadFile(f.DSLFile)
		if err != nil {
			return errors.New("reading DSL file: " + err.Error())
		}

		dslContent = string(bytes)
	}

	resp, err := f.repoTransaction.CreateDSL(f.OrganizationID, f.LedgerID, dslContent)
	if err != nil {
		return err
	}

	output.FormatAndPrint(f.factory, resp.ID, "Transaction", output.Created)

	return nil
}

func (f *factoryTransactionCreateDSL) setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.OrganizationID, "organization-id", "", "Specify the organization ID.")
	cmd.Flags().StringVar(&f.LedgerID, "ledger-id", "", "Specify the ledger ID.")
	cmd.Flags().StringVar(&f.DSLFile, "dsl-file", "", "Path to a DSL file containing transaction definition, or '-' for stdin.")
	cmd.Flags().BoolP("help", "h", false, "Displays more information about the Mdz CLI")
}

func newInjectFacCreateDSL(f *factory.Factory) *factoryTransactionCreateDSL {
	return &factoryTransactionCreateDSL{
		factory:         f,
		repoTransaction: rest.NewTransaction(f),
		tuiInput:        tui.Input,
	}
}

func newCmdTransactionCreateDSL(f *factoryTransactionCreateDSL) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-dsl",
		Short: "Creates a transaction using DSL.",
		Long: utils.Format(
			"Creates a new transaction in the specified ledger using a Domain Specific Language (DSL) file.",
			"The DSL format provides a more flexible way to define complex transactions.",
			"Returns a success or error message.",
		),
		Example: utils.Format(
			"$ mdz transaction create-dsl",
			"$ mdz transaction create-dsl -h",
			"$ mdz transaction create-dsl --dsl-file transaction.dsl",
			"$ cat transaction.dsl | mdz transaction create-dsl --dsl-file -",
		),
		RunE: f.runE,
	}

	f.setFlags(cmd)

	return cmd
}
