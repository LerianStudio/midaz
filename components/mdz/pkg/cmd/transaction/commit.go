package transaction

import (
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	rest "github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type commitOptions struct {
	organizationID string
	ledgerID       string
	transactionID  string
	dryRun         bool
	factory        rest.Factory
}

func (o *commitOptions) initFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.organizationID, "organization-id", "o", "", "Organization ID")
	cmd.Flags().StringVarP(&o.ledgerID, "ledger-id", "l", "", "Ledger ID")
	cmd.Flags().StringVarP(&o.transactionID, "transaction-id", "t", "", "Transaction ID")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "d", false, "Only print the command that would be executed")

	_ = cmd.MarkFlagRequired("organization-id")
	_ = cmd.MarkFlagRequired("ledger-id")
	_ = cmd.MarkFlagRequired("transaction-id")
}

func (o *commitOptions) validateOptions() error {
	if o.organizationID == "" {
		return fmt.Errorf("organization-id is required")
	}

	if o.ledgerID == "" {
		return fmt.Errorf("ledger-id is required")
	}

	if o.transactionID == "" {
		return fmt.Errorf("transaction-id is required")
	}

	// Validate UUIDs
	if _, err := uuid.Parse(o.organizationID); err != nil {
		return fmt.Errorf("invalid organization ID format: %v", err)
	}

	if _, err := uuid.Parse(o.ledgerID); err != nil {
		return fmt.Errorf("invalid ledger ID format: %v", err)
	}

	if _, err := uuid.Parse(o.transactionID); err != nil {
		return fmt.Errorf("invalid transaction ID format: %v", err)
	}

	return nil
}

func (o *commitOptions) run() (*mmodel.Transaction, error) {
	if err := o.validateOptions(); err != nil {
		return nil, err
	}

	if o.dryRun {
		fmt.Printf("Would commit transaction %s\n", o.transactionID)
		return nil, nil
	}

	return o.factory.Transaction().Commit(o.organizationID, o.ledgerID, o.transactionID)
}

type injectFacCommit struct {
	factory *factory.Factory
}

func (i *injectFacCommit) injectFactory() rest.Factory {
	return rest.NewFactory(i.factory)
}

func newInjectFacCommit(factory *factory.Factory) *injectFacCommit {
	return &injectFacCommit{
		factory: factory,
	}
}

func newCmdTransactionCommit(i *injectFacCommit) *cobra.Command {
	o := &commitOptions{
		factory: i.injectFactory(),
	}

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit a pending transaction",
		Long: utils.Format(
			"The commit command marks a transaction as committed, which finalizes",
			"the transaction and its associated operations.",
			"",
			"Once a transaction is committed, it cannot be reverted or deleted.",
		),
		Example: utils.Format(
			"$ mdz transaction commit --organization-id <id> --ledger-id <id> --transaction-id <id>",
			"$ mdz transaction commit -o <id> -l <id> -t <id>",
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
			fmt.Printf("Successfully committed transaction: %s\n", txnAPI.ID)
			fmt.Printf("Status: %s\n", txnAPI.Status.Code)
			return nil
		},
	}

	o.initFlags(cmd)
	return cmd
}