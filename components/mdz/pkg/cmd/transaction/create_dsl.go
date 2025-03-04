package transaction

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	rest "github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type createDSLOptions struct {
	organizationID         string
	ledgerID               string
	dslScript              string
	dslFile                string
	description            string
	chartOfAccountsGroup   string
	metadata               string
	dryRun                 bool
	idempotencyKey         string
	factory                rest.Factory
	createTransactionInput *mmodel.CreateTransactionDSLInput
}

func (o *createDSLOptions) initFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.organizationID, "organization-id", "o", "", "Organization ID")
	cmd.Flags().StringVarP(&o.ledgerID, "ledger-id", "l", "", "Ledger ID")
	cmd.Flags().StringVarP(&o.dslScript, "dsl", "d", "", "Transaction DSL script (inline)")
	cmd.Flags().StringVarP(&o.dslFile, "file", "f", "", "Path to file containing DSL script")
	cmd.Flags().StringVarP(&o.description, "description", "", "", "Transaction description")
	cmd.Flags().StringVarP(&o.chartOfAccountsGroup, "chart-group", "g", "", "Chart of accounts group name")
	cmd.Flags().StringVarP(&o.metadata, "metadata", "m", "", "Metadata in JSON format")
	cmd.Flags().StringVarP(&o.idempotencyKey, "idempotency-key", "i", "", "Idempotency key to prevent duplicate transactions")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "", false, "Only print the command that would be executed")

	_ = cmd.MarkFlagRequired("organization-id")
	_ = cmd.MarkFlagRequired("ledger-id")
}

func (o *createDSLOptions) validateOptions() error {
	if o.organizationID == "" {
		return fmt.Errorf("organization-id is required")
	}

	if o.ledgerID == "" {
		return fmt.Errorf("ledger-id is required")
	}

	// Either DSL script or DSL file is required
	if o.dslScript == "" && o.dslFile == "" {
		return fmt.Errorf("either --dsl or --file must be provided")
	}

	// Both DSL script and DSL file cannot be provided simultaneously
	if o.dslScript != "" && o.dslFile != "" {
		return fmt.Errorf("only one of --dsl or --file can be provided")
	}

	// If DSL file is provided, check if it exists
	if o.dslFile != "" {
		if _, err := os.Stat(o.dslFile); os.IsNotExist(err) {
			return fmt.Errorf("DSL file does not exist: %s", o.dslFile)
		}
	}

	// Parse and validate metadata if provided
	if o.metadata != "" {
		var metadataMap map[string]interface{}
		if err := json.Unmarshal([]byte(o.metadata), &metadataMap); err != nil {
			return fmt.Errorf("invalid metadata JSON: %v", err)
		}
	}

	// Validate UUIDs
	if _, err := uuid.Parse(o.organizationID); err != nil {
		return fmt.Errorf("invalid organization ID format: %v", err)
	}

	if _, err := uuid.Parse(o.ledgerID); err != nil {
		return fmt.Errorf("invalid ledger ID format: %v", err)
	}

	return nil
}

func (o *createDSLOptions) run() (*mmodel.Transaction, error) {
	if err := o.validateOptions(); err != nil {
		return nil, err
	}

	// Get DSL script from file if provided
	dslContent := o.dslScript
	if o.dslFile != "" {
		content, err := ioutil.ReadFile(o.dslFile)
		if err != nil {
			return nil, fmt.Errorf("error reading DSL file: %v", err)
		}
		dslContent = string(content)
	}

	o.createTransactionInput = &mmodel.CreateTransactionDSLInput{
		DSL:                  dslContent,
		Description:          o.description,
		ChartOfAccountsGroup: o.chartOfAccountsGroup,
	}

	// Add metadata if provided
	if o.metadata != "" {
		var metadataMap map[string]interface{}
		if err := json.Unmarshal([]byte(o.metadata), &metadataMap); err != nil {
			return nil, fmt.Errorf("invalid metadata JSON: %v", err)
		}
		o.createTransactionInput.Metadata = metadataMap
	}

	// Add idempotency key if provided
	if o.idempotencyKey != "" {
		o.createTransactionInput.IdempotencyKey = o.idempotencyKey
	}

	if o.dryRun {
		fmt.Printf("DSL Script:\n%s\n\n", dslContent)
		dslParams := map[string]interface{}{
			"Description":          o.description,
			"ChartOfAccountsGroup": o.chartOfAccountsGroup,
		}
		if o.metadata != "" {
			var metadataMap map[string]interface{}
			_ = json.Unmarshal([]byte(o.metadata), &metadataMap)
			dslParams["Metadata"] = metadataMap
		}
		if o.idempotencyKey != "" {
			dslParams["IdempotencyKey"] = o.idempotencyKey
		}
		paramsJSON, _ := json.MarshalIndent(dslParams, "", "  ")
		fmt.Printf("Parameters:\n%s\n", string(paramsJSON))
		return nil, nil
	}

	return o.factory.Transaction().CreateDSL(o.organizationID, o.ledgerID, *o.createTransactionInput)
}

type injectFacCreateDSL struct {
	factory *factory.Factory
}

func (i *injectFacCreateDSL) injectFactory() rest.Factory {
	return rest.NewFactory(i.factory)
}

func newInjectFacCreateDSL(factory *factory.Factory) *injectFacCreateDSL {
	return &injectFacCreateDSL{
		factory: factory,
	}
}

func newCmdTransactionCreateDSL(i *injectFacCreateDSL) *cobra.Command {
	o := &createDSLOptions{
		factory: i.injectFactory(),
	}

	cmd := &cobra.Command{
		Use:   "create-dsl",
		Short: "Create a new transaction using DSL syntax",
		Long: utils.Format(
			"The create-dsl command allows you to create a transaction using DSL syntax.",
			"You can provide the DSL script inline or through a file.",
			"",
			"DSL (Domain Specific Language) provides a more powerful way to define complex",
			"transactions compared to the standard JSON-based transaction creation.",
		),
		Example: utils.Format(
			"$ mdz transaction create-dsl --organization-id <id> --ledger-id <id> --dsl 'send 100.00 USD from account:src to account:dst'",
			"$ mdz transaction create-dsl -o <id> -l <id> -f transaction.dsl -g 'payments'",
			"$ mdz transaction create-dsl -o <id> -l <id> -d 'send 50 BTC from account:wallet to account:exchange' --metadata '{\"purpose\":\"withdrawal\"}'",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			txn, err := o.run()
			if err != nil {
				return err
			}

			if o.dryRun {
				return nil
			}

			// Convert to API transaction model for display
			txnAPI := model.AsTransaction(txn)
			fmt.Printf("Successfully created transaction: %s\n", txnAPI.ID)
			return nil
		},
	}

	o.initFlags(cmd)
	return cmd
}