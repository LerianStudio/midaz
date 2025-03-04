package balance

import (
	"encoding/json"
	"fmt"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	rest "github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/cmd/utils"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type createOptions struct {
	organizationID     string
	ledgerID           string
	accountID          string
	assetID            string
	initialAmount      int64
	initialScale       int32
	allowSending       bool
	allowReceiving     bool
	metadata           string
	dryRun             bool
	factory            rest.Factory
	createBalanceInput *mmodel.CreateBalanceInput
}

func (o *createOptions) initFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.organizationID, "organization-id", "o", "", "Organization ID")
	cmd.Flags().StringVarP(&o.ledgerID, "ledger-id", "l", "", "Ledger ID")
	cmd.Flags().StringVarP(&o.accountID, "account-id", "a", "", "Account ID")
	cmd.Flags().StringVarP(&o.assetID, "asset-id", "s", "", "Asset ID")
	cmd.Flags().Int64VarP(&o.initialAmount, "initial-amount", "i", 0, "Initial amount (requires initialScale)")
	cmd.Flags().Int32VarP(&o.initialScale, "initial-scale", "c", 0, "Initial scale (requires initialAmount)")
	cmd.Flags().BoolVarP(&o.allowSending, "allow-sending", "", true, "Allow sending from this balance")
	cmd.Flags().BoolVarP(&o.allowReceiving, "allow-receiving", "", true, "Allow receiving to this balance")
	cmd.Flags().StringVarP(&o.metadata, "metadata", "m", "", "Metadata in JSON format")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "d", false, "Only print the command that would be executed")

	_ = cmd.MarkFlagRequired("organization-id")
	_ = cmd.MarkFlagRequired("ledger-id")
	_ = cmd.MarkFlagRequired("account-id")
	_ = cmd.MarkFlagRequired("asset-id")
}

func (o *createOptions) validateOptions() error {
	if o.organizationID == "" {
		return fmt.Errorf("organization-id is required")
	}

	if o.ledgerID == "" {
		return fmt.Errorf("ledger-id is required")
	}

	if o.accountID == "" {
		return fmt.Errorf("account-id is required")
	}

	if o.assetID == "" {
		return fmt.Errorf("asset-id is required")
	}

	// If initial amount is set, initial scale must also be set
	if (o.initialAmount != 0 && o.initialScale == 0) || (o.initialAmount == 0 && o.initialScale != 0) {
		return fmt.Errorf("both initial-amount and initial-scale must be set together")
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

	if _, err := uuid.Parse(o.accountID); err != nil {
		return fmt.Errorf("invalid account ID format: %v", err)
	}

	if _, err := uuid.Parse(o.assetID); err != nil {
		return fmt.Errorf("invalid asset ID format: %v", err)
	}

	return nil
}

func (o *createOptions) run() (*mmodel.Balance, error) {
	if err := o.validateOptions(); err != nil {
		return nil, err
	}

	o.createBalanceInput = &mmodel.CreateBalanceInput{
		AssetID:        o.assetID,
		AllowSending:   &o.allowSending,
		AllowReceiving: &o.allowReceiving,
	}

	// Add initial amount and scale if provided
	if o.initialAmount != 0 && o.initialScale != 0 {
		o.createBalanceInput.InitialAmount = &o.initialAmount
		o.createBalanceInput.InitialScale = &o.initialScale
	}

	// Add metadata if provided
	if o.metadata != "" {
		var metadataMap map[string]interface{}
		if err := json.Unmarshal([]byte(o.metadata), &metadataMap); err != nil {
			return nil, fmt.Errorf("invalid metadata JSON: %v", err)
		}
		o.createBalanceInput.Metadata = metadataMap
	}

	if o.dryRun {
		inputJSON, _ := json.MarshalIndent(o.createBalanceInput, "", "  ")
		fmt.Printf("Balance that would be created:\n%s\n", string(inputJSON))
		return nil, nil
	}

	return o.factory.Balance().Create(o.organizationID, o.ledgerID, o.accountID, *o.createBalanceInput)
}

type injectFacCreate struct {
	factory *factory.Factory
}

func (i *injectFacCreate) injectFactory() rest.Factory {
	return rest.NewFactory(i.factory)
}

func newInjectFacCreate(factory *factory.Factory) *injectFacCreate {
	return &injectFacCreate{
		factory: factory,
	}
}

func newCmdBalanceCreate(i *injectFacCreate) *cobra.Command {
	o := &createOptions{
		factory: i.injectFactory(),
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new balance for an account",
		Long: utils.Format(
			"The create command allows you to create a new balance for an account.",
			"You need to specify the organization ID, ledger ID, account ID, and asset ID.",
			"You can also set initial amount and scale, allow sending/receiving flags, and metadata.",
		),
		Example: utils.Format(
			"$ mdz balance create --organization-id <id> --ledger-id <id> --account-id <id> --asset-id <id>",
			"$ mdz balance create -o <id> -l <id> -a <id> -s <id> --initial-amount 1000 --initial-scale 2",
			"$ mdz balance create -o <id> -l <id> -a <id> -s <id> --allow-sending=false --metadata '{\"description\":\"Savings balance\"}'",
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			bal, err := o.run()
			if err != nil {
				return err
			}

			if o.dryRun {
				return nil
			}

			// Convert to API balance model for display
			balanceAPI := model.AsBalance(bal)
			fmt.Printf("Successfully created balance: %s\n", balanceAPI.ID)
			return nil
		},
	}

	o.initFlags(cmd)
	return cmd
}