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

type revertOptions struct {
	organizationID string
	ledgerID       string
	transactionID  string
	dryRun         bool
	factory        rest.Factory
}

func (o *revertOptions) initFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.organizationID, "organization-id", "o", "", "Organization ID")
	cmd.Flags().StringVarP(&o.ledgerID, "ledger-id", "l", "", "Ledger ID")
	cmd.Flags().StringVarP(&o.transactionID, "transaction-id", "t", "", "Transaction ID")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "d", false, "Only print the command that would be executed")

	_ = cmd.MarkFlagRequired("organization-id")
	_ = cmd.MarkFlagRequired("ledger-id")
	_ = cmd.MarkFlagRequired("transaction-id")
}

func (o *revertOptions) validateOptions() error {
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

func (o *revertOptions) run() (*mmodel.Transaction, error) {
	if err := o.validateOptions(); err != nil {
		return nil, err
	}

	if o.dryRun {
		fmt.Printf("Would revert transaction %s\n", o.transactionID)
		return nil, nil
	}

	return o.factory.Transaction().Revert(o.organizationID, o.ledgerID, o.transactionID)
}

type injectFacRevert struct {
	factory *factory.Factory
}

func (i *injectFacRevert) injectFactory() rest.Factory {
	return rest.NewFactory(i.factory)
}

func newInjectFacRevert(factory *factory.Factory) *injectFacRevert {
	return &injectFacRevert{
		factory: factory,
	}
}

func newCmdTransactionRevert(i *injectFacRevert) *cobra.Command {
	o := &revertOptions{
		factory: i.injectFactory(),
	}

	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Revert a pending transaction",
		Long: utils.Format(
			"The revert command marks a transaction as reverted, which cancels",
			"the transaction and its associated operations without deleting them.",
			"",
			"This provides an audit trail of attempted transactions that were not completed.",
		),
		Example: utils.Format(
			"$ mdz transaction revert --organization-id <id> --ledger-id <id> --transaction-id <id>",
			"$ mdz transaction revert -o <id> -l <id> -t <id>",
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
			fmt.Printf("Successfully reverted transaction: %s\n", txnAPI.ID)
			fmt.Printf("Status: %s\n", txnAPI.Status.Code)
			return nil
		},
	}

	o.initFlags(cmd)
	return cmd
}